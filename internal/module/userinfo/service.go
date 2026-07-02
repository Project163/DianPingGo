package userinfo

import "context"

type UserRepository interface {
	GetUserInfoByUserID(ctx context.Context, userID uint64) (*UserInfo, error)
	CreateUserInfo(ctx context.Context, userInfoDTO *UserInfoDTO) error
}

type Service struct {
	repo UserRepository
}

func NewService(repo UserRepository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) GetUserInfoByUserID(ctx context.Context, userID uint64) (*UserInfoDTO, error) {
	var userInfo *UserInfo
	userInfo, err := s.repo.GetUserInfoByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userInfo == nil {
		return nil, nil
	}
	return s.toUserInfoDTO(userInfo), nil
}

func (s *Service) CreateUserInfo(ctx context.Context, userInfoDTO *UserInfoDTO) error {
	return s.repo.CreateUserInfo(ctx, userInfoDTO)
}

func (s *Service) toUserInfoDTO(userInfo *UserInfo) *UserInfoDTO {
	return &UserInfoDTO{
		UserID:    userInfo.UserID,
		City:      userInfo.City,
		Introduce: userInfo.Introduce,
		Fans:      userInfo.Fans,
		Followee:  userInfo.Followee,
		Gender:    userInfo.Gender,
		Birthday:  userInfo.Birthday,
		Credits:   userInfo.Credits,
		Level:     userInfo.Level,
	}
}
