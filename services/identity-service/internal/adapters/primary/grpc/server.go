package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/ports"
)

// Server adapte le port gRPC vers le port primaire du domaine.
type Server struct {
	identityv1.UnimplementedIdentityServiceServer // Obligatoire pour la compatibilité forward
	service                                       ports.IdentityService
}

// RegisterTo permet d'enregistrer ce handler sur un serveur gRPC existant
func (s *Server) RegisterTo(grpcServer *grpc.Server) {
	identityv1.RegisterIdentityServiceServer(grpcServer, s)
}

// NewAuthGrpcServer initialise le serveur gRPC
func NewAuthGrpcServer(service ports.IdentityService) *Server {
	return &Server{service: service}
}

// Register (Implemente IdentityServiceServer)
func (s *Server) Register(ctx context.Context, req *identityv1.RegisterRequest) (*identityv1.RegisterResponse, error) {
	// 1. Mapping Proto -> Domain Cmd
	cmd := ports.RegisterCmd{
		Email:    req.Email,
		Password: req.Password,
		Username: req.Username,
		FullName: req.FullName,
	}

	// 2. Appel Service
	authResponse, err := s.service.Register(ctx, cmd)
	if err != nil {
		return nil, mapDomainError(err)
	}

	// 3. Mapping Domain -> Proto
	return &identityv1.RegisterResponse{
		User:             mapUserToProto(authResponse.User),
		AccessToken:      authResponse.AccessToken,
		RefreshToken:     authResponse.RefreshToken,
		ExpiresInSeconds: int64(authResponse.ExpiresIn.Seconds()),
	}, nil
}

// Login
func (s *Server) Login(ctx context.Context, req *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	cmd := ports.LoginCmd{
		Email:    req.Email,
		Password: req.Password,
		IP:       req.IpAddress,
		Device:   req.DeviceInfo,
	}

	authResponse, err := s.service.Login(ctx, cmd)
	if err != nil {
		return nil, mapDomainError(err)
	}

	return &identityv1.LoginResponse{
		User:             mapUserToProto(authResponse.User),
		AccessToken:      authResponse.AccessToken,
		RefreshToken:     authResponse.RefreshToken,
		ExpiresInSeconds: int64(authResponse.ExpiresIn.Seconds()),
	}, nil
}

// GetUser
func (s *Server) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	user, err := s.service.GetUser(ctx, req.UserId)
	if err != nil {
		return nil, mapDomainError(err)
	}

	return &identityv1.GetUserResponse{
		User: mapUserToProto(user),
	}, nil
}

// UpdateProfile
func (s *Server) UpdateProfile(ctx context.Context, req *identityv1.UpdateProfileRequest) (*identityv1.UpdateProfileResponse, error) {
	// L'utilisation de 'optional' dans le proto génère des pointeurs (*string) en Go.
	// C'est parfait, car notre UpdateProfileCmd attend aussi des pointeurs !
	cmd := ports.UpdateProfileCmd{
		UserID:   req.UserId,
		FullName: req.FullName, // Type: *string
		Email:    req.Email,    // Type: *string
	}

	updatedUser, err := s.service.UpdateProfile(ctx, cmd)
	if err != nil {
		return nil, mapDomainError(err)
	}

	return &identityv1.UpdateProfileResponse{
		User: mapUserToProto(updatedUser),
	}, nil
}

// ChangePassword
func (s *Server) ChangePassword(ctx context.Context, req *identityv1.ChangePasswordRequest) (*emptypb.Empty, error) {
	err := s.service.ChangePassword(ctx, req.UserId, req.OldPassword, req.NewPassword)
	if err != nil {
		return nil, mapDomainError(err)
	}
	return &emptypb.Empty{}, nil
}

// ValidateToken
func (s *Server) ValidateToken(ctx context.Context, req *identityv1.ValidateTokenRequest) (*identityv1.ValidateTokenResponse, error) {
	userID, err := s.service.ValidateToken(ctx, req.Token)
	if err != nil {
		// Ici, on ne renvoie pas forcément une erreur gRPC, mais une réponse valide disant "faux"
		// Ou alors on renvoie Unauthenticated. C'est un choix d'API.
		// Choisissons de renvoyer une réponse explicite :
		return &identityv1.ValidateTokenResponse{
			IsValid: false,
		}, nil
	}

	return &identityv1.ValidateTokenResponse{
		IsValid: true,
		UserId:  userID,
	}, nil
}

// RefreshToken (Placeholder, à implémenter si le service le supporte)
func (s *Server) RefreshToken(ctx context.Context, req *identityv1.RefreshTokenRequest) (*identityv1.RefreshTokenResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented yet")
}

// Listen est un helper pour démarrer le serveur dans le main.go
func (s *Server) Listen(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	// Utilisation d'options gRPC recommandées (Interceptors à venir)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, s)

	fmt.Printf("🚀 Identity gRPC Server listening on %s\n", address)
	return grpcServer.Serve(lis)
}

// --- HELPERS DE MAPPING ---

// mapUserToProto convertit l'entité Domain vers le message Proto
func mapUserToProto(u *domain.User) *identityv1.User {
	if u == nil {
		return nil
	}
	return &identityv1.User{
		Id:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		FullName:  u.FullName,
		IsActive:  u.IsActive,
		CreatedAt: timestamppb.New(u.CreatedAt),
		UpdatedAt: timestamppb.New(u.UpdatedAt),
	}
}

// mapDomainError traduit les erreurs métier en codes d'erreur gRPC standard
func mapDomainError(err error) error {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrInvalidToken):
		return status.Error(codes.Unauthenticated, "invalid token")
	case errors.Is(err, domain.ErrInvalidEmail) || errors.Is(err, domain.ErrInvalidUsername):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		// Erreur interne (DB down, etc.) -> ne pas fuiter les détails techniques
		return status.Error(codes.Internal, "internal server error")
	}
}
