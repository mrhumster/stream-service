package models

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrStreamTitleRequired  = errors.New("title is required")
	ErrStreamTitleIsTooLong = errors.New("title is too long")
	ErrInvalidStatus        = errors.New("invalid stream stastus")
	ErrInvalidVisibility    = errors.New("invalid stream visibility")
	ErrOwnerIDRequired      = errors.New("owner id required")
)

func (s *Stream) Validate() error {
	if err := s.validateTitle(); err != nil {
		return err
	}

	if err := s.validateVisibility(); err != nil {
		return err
	}

	if err := s.validateStatus(); err != nil {
		return err
	}

	if err := s.validateOwnerID(); err != nil {
		return err
	}
	return nil
}

func (s *Stream) validateStatus() error {
	switch s.Status {
	case StatusDraft, StatusProcessing, StatusPublished, StatusReady, StatusError:
		return nil
	default:
		return ErrInvalidStatus
	}
}

func (s *Stream) validateTitle() error {
	if s.Title == "" {
		return ErrStreamTitleRequired
	}
	if len(s.Title) > 255 {
		return ErrStreamTitleIsTooLong
	}
	return nil
}

func (s *Stream) validateVisibility() error {
	switch s.Visibility {
	case VisibilityPrivate, VisibilityPublic, VisibilityUnlisted:
		return nil
	default:
		return ErrInvalidVisibility
	}
}

func (s *Stream) validateOwnerID() error {
	if s.OwnerID == uuid.Nil {
		return ErrOwnerIDRequired
	}
	return nil
}
