package models

import "errors"

type Session struct {
	Info  *SessionInfo
	Turns *TurnBuffer
}

func (s *Session) Validate() error {
	if s == nil {
		return errors.New("session is nil")
	}

	if s.Info == nil {
		return errors.New("session meta must not be nil")
	}

	if err := s.Info.Validate(); err != nil {
		return err
	}

	if s.Turns == nil {
		return errors.New("turns must not be nil")
	}

	return nil
}
