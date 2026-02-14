package v1

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

func (s *APIV1Service) applyMemoVisibilityFilter(ctx context.Context, memoFind *store.FindMemo) error {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get user")
	}
	if currentUser == nil {
		memoFind.VisibilityList = []store.Visibility{store.Public}
		return nil
	}

	if memoFind.CreatorID == nil {
		filter := fmt.Sprintf(`creator_id == %d || visibility in ["PUBLIC", "PROTECTED"]`, currentUser.ID)
		memoFind.Filters = append(memoFind.Filters, filter)
		return nil
	}
	if *memoFind.CreatorID != currentUser.ID {
		memoFind.VisibilityList = []store.Visibility{store.Public, store.Protected}
	}
	return nil
}

func (s *APIV1Service) convertMemoListToMessages(ctx context.Context, memos []*store.Memo) ([]*v1pb.Memo, error) {
	if len(memos) == 0 {
		return []*v1pb.Memo{}, nil
	}

	reactionMap := make(map[string][]*store.Reaction)
	contentIDList := make([]string, 0, len(memos))

	attachmentMap := make(map[int32][]*store.Attachment)
	memoIDList := make([]int32, 0, len(memos))

	for _, memo := range memos {
		contentIDList = append(contentIDList, fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID))
		memoIDList = append(memoIDList, memo.ID)
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{ContentIDList: contentIDList})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}
	for _, reaction := range reactions {
		reactionMap[reaction.ContentID] = append(reactionMap[reaction.ContentID], reaction)
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{MemoIDList: memoIDList})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}
	for _, attachment := range attachments {
		attachmentMap[*attachment.MemoID] = append(attachmentMap[*attachment.MemoID], attachment)
	}

	memoMessages := make([]*v1pb.Memo, 0, len(memos))
	for _, memo := range memos {
		memoName := fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID)
		reactions := reactionMap[memoName]
		attachments := attachmentMap[memo.ID]

		memoMessage, err := s.convertMemoFromStore(ctx, memo, reactions, attachments)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert memo")
		}
		memoMessages = append(memoMessages, memoMessage)
	}

	return memoMessages, nil
}
