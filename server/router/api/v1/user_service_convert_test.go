package v1

import (
	"testing"

	storepb "github.com/usememos/memos/proto/gen/store"
)

func TestConvertUserSettingFromStoreUnsupportedKeyReturnsNil(t *testing.T) {
	setting := &storepb.UserSetting{
		UserId: 1,
		Key:    storepb.UserSetting_REFRESH_TOKENS,
	}

	result := convertUserSettingFromStore(setting, 1, setting.Key)
	if result != nil {
		t.Fatalf("expected nil for unsupported key, got %+v", result)
	}
}

func TestConvertUserSettingFromStoreGeneralReturnsValue(t *testing.T) {
	setting := &storepb.UserSetting{
		UserId: 1,
		Key:    storepb.UserSetting_GENERAL,
		Value: &storepb.UserSetting_General{
			General: &storepb.GeneralUserSetting{
				Locale:         "zh-Hans",
				MemoVisibility: "PRIVATE",
				Theme:          "system",
			},
		},
	}

	result := convertUserSettingFromStore(setting, 1, setting.Key)
	if result == nil {
		t.Fatal("expected non-nil result for general setting")
	}
	general := result.GetGeneralSetting()
	if general == nil {
		t.Fatal("expected general setting in result")
	}
	if general.Locale != "zh-Hans" {
		t.Fatalf("expected locale zh-Hans, got %q", general.Locale)
	}
}
