package v1

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// GetInstanceProfile returns the instance profile.
func (s *APIV1Service) GetInstanceProfile(ctx context.Context, _ *v1pb.GetInstanceProfileRequest) (*v1pb.InstanceProfile, error) {
	admin, err := s.GetInstanceAdmin(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get instance admin: %v", err)
	}

	instanceProfile := &v1pb.InstanceProfile{
		Version:     s.Profile.Version,
		Demo:        s.Profile.Demo,
		InstanceUrl: s.Profile.InstanceURL,
		Admin:       admin, // nil when not initialized
	}
	return instanceProfile, nil
}

func (s *APIV1Service) GetInstanceSetting(ctx context.Context, request *v1pb.GetInstanceSettingRequest) (*v1pb.InstanceSetting, error) {
	instanceSettingKeyString, err := ExtractInstanceSettingKeyFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid instance setting name: %v", err)
	}

	instanceSettingKey := storepb.InstanceSettingKey(storepb.InstanceSettingKey_value[instanceSettingKeyString])
	// Get instance setting from store with default value.
	switch instanceSettingKey {
	case storepb.InstanceSettingKey_BASIC:
		_, err = s.Store.GetInstanceBasicSetting(ctx)
	case storepb.InstanceSettingKey_GENERAL:
		_, err = s.Store.GetInstanceGeneralSetting(ctx)
	case storepb.InstanceSettingKey_MEMO_RELATED:
		_, err = s.Store.GetInstanceMemoRelatedSetting(ctx)
	case storepb.InstanceSettingKey_STORAGE:
		_, err = s.Store.GetInstanceStorageSetting(ctx)
	case storepb.InstanceSettingKey_AI:
		_, err = s.Store.GetInstanceAISetting(ctx)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported instance setting key: %v", instanceSettingKey)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get instance setting: %v", err)
	}

	instanceSetting, err := s.Store.GetInstanceSetting(ctx, &store.FindInstanceSetting{
		Name: instanceSettingKey.String(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get instance setting: %v", err)
	}
	if instanceSetting == nil {
		return nil, status.Errorf(codes.NotFound, "instance setting not found")
	}

	// For sensitive settings, only admin can get it.
	if instanceSetting.Key == storepb.InstanceSettingKey_STORAGE || instanceSetting.Key == storepb.InstanceSettingKey_AI {
		user, err := s.fetchCurrentUser(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
		}
		if user == nil {
			return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
		}
		if user.Role != store.RoleAdmin {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
	}

	return convertInstanceSettingFromStore(instanceSetting), nil
}

func (s *APIV1Service) UpdateInstanceSetting(ctx context.Context, request *v1pb.UpdateInstanceSettingRequest) (*v1pb.InstanceSetting, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if user.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	// TODO: Apply update_mask if specified
	_ = request.UpdateMask

	if request.Setting == nil {
		return nil, status.Errorf(codes.InvalidArgument, "setting is required")
	}
	settingKeyString, err := ExtractInstanceSettingKeyFromName(request.Setting.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid instance setting name: %v", err)
	}
	settingKeyValue, ok := storepb.InstanceSettingKey_value[settingKeyString]
	if !ok || storepb.InstanceSettingKey(settingKeyValue) == storepb.InstanceSettingKey_INSTANCE_SETTING_KEY_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported instance setting key: %s", settingKeyString)
	}
	settingKey := storepb.InstanceSettingKey(settingKeyValue)

	var instanceSetting *storepb.InstanceSetting
	switch settingKey {
	case storepb.InstanceSettingKey_AI:
		instanceSetting, err = s.upsertInstanceAISetting(ctx, request.Setting.GetAiSetting())
	default:
		updateSetting := convertInstanceSettingToStore(request.Setting)
		instanceSetting, err = s.Store.UpsertInstanceSetting(ctx, updateSetting)
	}
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
			return nil, err
		}
		return nil, status.Errorf(codes.Internal, "failed to upsert instance setting: %v", err)
	}

	return convertInstanceSettingFromStore(instanceSetting), nil
}

func convertInstanceSettingFromStore(setting *storepb.InstanceSetting) *v1pb.InstanceSetting {
	instanceSetting := &v1pb.InstanceSetting{
		Name: fmt.Sprintf("instance/settings/%s", setting.Key.String()),
	}
	switch setting.Value.(type) {
	case *storepb.InstanceSetting_GeneralSetting:
		instanceSetting.Value = &v1pb.InstanceSetting_GeneralSetting_{
			GeneralSetting: convertInstanceGeneralSettingFromStore(setting.GetGeneralSetting()),
		}
	case *storepb.InstanceSetting_StorageSetting:
		instanceSetting.Value = &v1pb.InstanceSetting_StorageSetting_{
			StorageSetting: convertInstanceStorageSettingFromStore(setting.GetStorageSetting()),
		}
	case *storepb.InstanceSetting_MemoRelatedSetting:
		instanceSetting.Value = &v1pb.InstanceSetting_MemoRelatedSetting_{
			MemoRelatedSetting: convertInstanceMemoRelatedSettingFromStore(setting.GetMemoRelatedSetting()),
		}
	case *storepb.InstanceSetting_AiSetting:
		instanceSetting.Value = &v1pb.InstanceSetting_AiSetting{
			AiSetting: convertInstanceAISettingFromStore(setting.GetAiSetting()),
		}
	}
	return instanceSetting
}

func convertInstanceSettingToStore(setting *v1pb.InstanceSetting) *storepb.InstanceSetting {
	settingKeyString, _ := ExtractInstanceSettingKeyFromName(setting.Name)
	instanceSetting := &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey(storepb.InstanceSettingKey_value[settingKeyString]),
		Value: &storepb.InstanceSetting_GeneralSetting{
			GeneralSetting: convertInstanceGeneralSettingToStore(setting.GetGeneralSetting()),
		},
	}
	switch instanceSetting.Key {
	case storepb.InstanceSettingKey_GENERAL:
		instanceSetting.Value = &storepb.InstanceSetting_GeneralSetting{
			GeneralSetting: convertInstanceGeneralSettingToStore(setting.GetGeneralSetting()),
		}
	case storepb.InstanceSettingKey_STORAGE:
		instanceSetting.Value = &storepb.InstanceSetting_StorageSetting{
			StorageSetting: convertInstanceStorageSettingToStore(setting.GetStorageSetting()),
		}
	case storepb.InstanceSettingKey_MEMO_RELATED:
		instanceSetting.Value = &storepb.InstanceSetting_MemoRelatedSetting{
			MemoRelatedSetting: convertInstanceMemoRelatedSettingToStore(setting.GetMemoRelatedSetting()),
		}
	case storepb.InstanceSettingKey_AI:
		instanceSetting.Value = &storepb.InstanceSetting_AiSetting{
			AiSetting: convertInstanceAISettingToStore(setting.GetAiSetting()),
		}
	default:
		// Keep the default GeneralSetting value
	}
	return instanceSetting
}

func convertInstanceGeneralSettingFromStore(setting *storepb.InstanceGeneralSetting) *v1pb.InstanceSetting_GeneralSetting {
	if setting == nil {
		return nil
	}

	generalSetting := &v1pb.InstanceSetting_GeneralSetting{
		DisallowUserRegistration: setting.DisallowUserRegistration,
		DisallowPasswordAuth:     setting.DisallowPasswordAuth,
		AdditionalScript:         setting.AdditionalScript,
		AdditionalStyle:          setting.AdditionalStyle,
		WeekStartDayOffset:       setting.WeekStartDayOffset,
		DisallowChangeUsername:   setting.DisallowChangeUsername,
		DisallowChangeNickname:   setting.DisallowChangeNickname,
	}
	if setting.CustomProfile != nil {
		generalSetting.CustomProfile = &v1pb.InstanceSetting_GeneralSetting_CustomProfile{
			Title:       setting.CustomProfile.Title,
			Description: setting.CustomProfile.Description,
			LogoUrl:     setting.CustomProfile.LogoUrl,
		}
	}
	return generalSetting
}

func convertInstanceGeneralSettingToStore(setting *v1pb.InstanceSetting_GeneralSetting) *storepb.InstanceGeneralSetting {
	if setting == nil {
		return nil
	}
	generalSetting := &storepb.InstanceGeneralSetting{
		DisallowUserRegistration: setting.DisallowUserRegistration,
		DisallowPasswordAuth:     setting.DisallowPasswordAuth,
		AdditionalScript:         setting.AdditionalScript,
		AdditionalStyle:          setting.AdditionalStyle,
		WeekStartDayOffset:       setting.WeekStartDayOffset,
		DisallowChangeUsername:   setting.DisallowChangeUsername,
		DisallowChangeNickname:   setting.DisallowChangeNickname,
	}
	if setting.CustomProfile != nil {
		generalSetting.CustomProfile = &storepb.InstanceCustomProfile{
			Title:       setting.CustomProfile.Title,
			Description: setting.CustomProfile.Description,
			LogoUrl:     setting.CustomProfile.LogoUrl,
		}
	}
	return generalSetting
}

func convertInstanceStorageSettingFromStore(settingpb *storepb.InstanceStorageSetting) *v1pb.InstanceSetting_StorageSetting {
	if settingpb == nil {
		return nil
	}
	setting := &v1pb.InstanceSetting_StorageSetting{
		StorageType:       v1pb.InstanceSetting_StorageSetting_StorageType(settingpb.StorageType),
		FilepathTemplate:  settingpb.FilepathTemplate,
		UploadSizeLimitMb: settingpb.UploadSizeLimitMb,
	}
	if settingpb.S3Config != nil {
		setting.S3Config = &v1pb.InstanceSetting_StorageSetting_S3Config{
			AccessKeyId:     settingpb.S3Config.AccessKeyId,
			AccessKeySecret: settingpb.S3Config.AccessKeySecret,
			Endpoint:        settingpb.S3Config.Endpoint,
			Region:          settingpb.S3Config.Region,
			Bucket:          settingpb.S3Config.Bucket,
			UsePathStyle:    settingpb.S3Config.UsePathStyle,
		}
	}
	return setting
}

func convertInstanceStorageSettingToStore(setting *v1pb.InstanceSetting_StorageSetting) *storepb.InstanceStorageSetting {
	if setting == nil {
		return nil
	}
	settingpb := &storepb.InstanceStorageSetting{
		StorageType:       storepb.InstanceStorageSetting_StorageType(setting.StorageType),
		FilepathTemplate:  setting.FilepathTemplate,
		UploadSizeLimitMb: setting.UploadSizeLimitMb,
	}
	if setting.S3Config != nil {
		settingpb.S3Config = &storepb.StorageS3Config{
			AccessKeyId:     setting.S3Config.AccessKeyId,
			AccessKeySecret: setting.S3Config.AccessKeySecret,
			Endpoint:        setting.S3Config.Endpoint,
			Region:          setting.S3Config.Region,
			Bucket:          setting.S3Config.Bucket,
			UsePathStyle:    setting.S3Config.UsePathStyle,
		}
	}
	return settingpb
}

func convertInstanceMemoRelatedSettingFromStore(setting *storepb.InstanceMemoRelatedSetting) *v1pb.InstanceSetting_MemoRelatedSetting {
	if setting == nil {
		return nil
	}
	return &v1pb.InstanceSetting_MemoRelatedSetting{
		DisallowPublicVisibility: setting.DisallowPublicVisibility,
		DisplayWithUpdateTime:    setting.DisplayWithUpdateTime,
		ContentLengthLimit:       setting.ContentLengthLimit,
		EnableDoubleClickEdit:    setting.EnableDoubleClickEdit,
		Reactions:                setting.Reactions,
	}
}

func convertInstanceMemoRelatedSettingToStore(setting *v1pb.InstanceSetting_MemoRelatedSetting) *storepb.InstanceMemoRelatedSetting {
	if setting == nil {
		return nil
	}
	return &storepb.InstanceMemoRelatedSetting{
		DisallowPublicVisibility: setting.DisallowPublicVisibility,
		DisplayWithUpdateTime:    setting.DisplayWithUpdateTime,
		ContentLengthLimit:       setting.ContentLengthLimit,
		EnableDoubleClickEdit:    setting.EnableDoubleClickEdit,
		Reactions:                setting.Reactions,
	}
}

func convertInstanceAISettingFromStore(setting *storepb.InstanceAISetting) *v1pb.InstanceSetting_AISetting {
	if setting == nil {
		return nil
	}
	models, selectedModel := normalizeEmbeddingModels(setting.OpenaiEmbeddingModels, setting.OpenaiEmbeddingModel)
	return &v1pb.InstanceSetting_AISetting{
		OpenaiBaseUrl:                 setting.OpenaiBaseUrl,
		OpenaiEmbeddingModel:          selectedModel,
		OpenaiEmbeddingModels:         models,
		OpenaiApiKeySet:               setting.OpenaiApiKeyEncrypted != "",
		OpenaiEmbeddingMaxRetry:       setting.OpenaiEmbeddingMaxRetry,
		OpenaiEmbeddingRetryBackoffMs: setting.OpenaiEmbeddingRetryBackoffMs,
		SemanticEmbeddingConcurrency:  setting.SemanticEmbeddingConcurrency,
		SemanticReindexRunning:        setting.SemanticReindexRunning,
		SemanticReindexTotal:          setting.SemanticReindexTotal,
		SemanticReindexProcessed:      setting.SemanticReindexProcessed,
		SemanticReindexFailed:         setting.SemanticReindexFailed,
		SemanticReindexStartedTs:      setting.SemanticReindexStartedTs,
		SemanticReindexUpdatedTs:      setting.SemanticReindexUpdatedTs,
		SemanticReindexModel:          setting.SemanticReindexModel,
	}
}

func convertInstanceAISettingToStore(setting *v1pb.InstanceSetting_AISetting) *storepb.InstanceAISetting {
	if setting == nil {
		return nil
	}
	models, selectedModel := normalizeEmbeddingModels(setting.OpenaiEmbeddingModels, setting.OpenaiEmbeddingModel)
	return &storepb.InstanceAISetting{
		OpenaiBaseUrl:                 setting.OpenaiBaseUrl,
		OpenaiEmbeddingModel:          selectedModel,
		OpenaiEmbeddingModels:         models,
		OpenaiEmbeddingMaxRetry:       setting.OpenaiEmbeddingMaxRetry,
		OpenaiEmbeddingRetryBackoffMs: setting.OpenaiEmbeddingRetryBackoffMs,
		SemanticEmbeddingConcurrency:  setting.SemanticEmbeddingConcurrency,
		SemanticReindexRunning:        setting.SemanticReindexRunning,
		SemanticReindexTotal:          setting.SemanticReindexTotal,
		SemanticReindexProcessed:      setting.SemanticReindexProcessed,
		SemanticReindexFailed:         setting.SemanticReindexFailed,
		SemanticReindexStartedTs:      setting.SemanticReindexStartedTs,
		SemanticReindexUpdatedTs:      setting.SemanticReindexUpdatedTs,
		SemanticReindexModel:          setting.SemanticReindexModel,
	}
}

func (s *APIV1Service) upsertInstanceAISetting(ctx context.Context, setting *v1pb.InstanceSetting_AISetting) (*storepb.InstanceSetting, error) {
	existingSetting, err := s.Store.GetInstanceAISetting(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get existing instance ai setting")
	}

	updatedSetting := &storepb.InstanceAISetting{}
	if existingSetting != nil {
		updatedSetting.OpenaiApiKeyEncrypted = existingSetting.OpenaiApiKeyEncrypted
		updatedSetting.SemanticReindexRunning = existingSetting.SemanticReindexRunning
		updatedSetting.SemanticReindexTotal = existingSetting.SemanticReindexTotal
		updatedSetting.SemanticReindexProcessed = existingSetting.SemanticReindexProcessed
		updatedSetting.SemanticReindexFailed = existingSetting.SemanticReindexFailed
		updatedSetting.SemanticReindexStartedTs = existingSetting.SemanticReindexStartedTs
		updatedSetting.SemanticReindexUpdatedTs = existingSetting.SemanticReindexUpdatedTs
		updatedSetting.SemanticReindexModel = existingSetting.SemanticReindexModel
	}
	if setting != nil {
		if err := validateInstanceAISetting(setting); err != nil {
			return nil, err
		}
		models, selectedModel := normalizeEmbeddingModels(setting.OpenaiEmbeddingModels, setting.OpenaiEmbeddingModel)
		updatedSetting.OpenaiBaseUrl = strings.TrimSpace(setting.OpenaiBaseUrl)
		updatedSetting.OpenaiEmbeddingModel = selectedModel
		updatedSetting.OpenaiEmbeddingModels = models
		updatedSetting.OpenaiEmbeddingMaxRetry = setting.OpenaiEmbeddingMaxRetry
		updatedSetting.OpenaiEmbeddingRetryBackoffMs = setting.OpenaiEmbeddingRetryBackoffMs
		updatedSetting.SemanticEmbeddingConcurrency = setting.SemanticEmbeddingConcurrency
		if setting.ClearOpenaiApiKey {
			updatedSetting.OpenaiApiKeyEncrypted = ""
		}
		if openaiAPIKey := strings.TrimSpace(setting.OpenaiApiKey); openaiAPIKey != "" {
			encryptedAPIKey, err := encryptSensitiveValue(s.Secret, openaiAPIKey)
			if err != nil {
				return nil, errors.Wrap(err, "failed to encrypt openai api key")
			}
			updatedSetting.OpenaiApiKeyEncrypted = encryptedAPIKey
		}
	}

	instanceSetting, err := s.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key:   storepb.InstanceSettingKey_AI,
		Value: &storepb.InstanceSetting_AiSetting{AiSetting: updatedSetting},
	})
	if err != nil {
		return nil, err
	}

	// Apply updated embedding concurrency immediately for subsequent async sync jobs.
	s.setEmbeddingSemaphoreLimit(resolveEmbeddingRefreshConcurrencyWithSetting(updatedSetting.SemanticEmbeddingConcurrency))

	if setting != nil && setting.TriggerSemanticReindex {
		if err := s.startSemanticReindexTask(ctx); err != nil {
			return nil, err
		}
	}
	return instanceSetting, nil
}

func validateInstanceAISetting(setting *v1pb.InstanceSetting_AISetting) error {
	if setting == nil {
		return nil
	}

	if setting.OpenaiEmbeddingMaxRetry < 0 {
		return status.Errorf(codes.InvalidArgument, "openai_embedding_max_retry must be non-negative")
	}
	if setting.OpenaiEmbeddingRetryBackoffMs < 0 {
		return status.Errorf(codes.InvalidArgument, "openai_embedding_retry_backoff_ms must be non-negative")
	}
	if setting.SemanticEmbeddingConcurrency < 0 {
		return status.Errorf(codes.InvalidArgument, "semantic_embedding_concurrency must be non-negative")
	}
	return nil
}

func normalizeEmbeddingModels(rawModels []string, selectedModel string) ([]string, string) {
	seen := make(map[string]struct{}, len(rawModels)+1)
	models := make([]string, 0, len(rawModels)+1)

	appendModel := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		models = append(models, trimmed)
	}

	appendModel(selectedModel)
	for _, model := range rawModels {
		appendModel(model)
	}

	finalSelected := strings.TrimSpace(selectedModel)
	if finalSelected == "" && len(models) > 0 {
		finalSelected = models[0]
	}
	return models, finalSelected
}

func (s *APIV1Service) GetInstanceAdmin(ctx context.Context) (*v1pb.User, error) {
	adminUserType := store.RoleAdmin
	user, err := s.Store.GetUser(ctx, &store.FindUser{
		Role: &adminUserType,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find admin")
	}
	if user == nil {
		return nil, nil
	}

	return convertUserFromStore(user), nil
}
