package handler

import (
	"errors"
	"fmt"
)

type State struct {
	CwdHandle    File
	ROFile       File
	CDSectorSize int // of ROFile, used by ReadCD2048Critical
	WOFile       WritableFile
}

func (s *State) Close() error {
	var errs []error

	if s.ROFile != nil {
		if err := s.ROFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ROFile close failed: %w", err))
		}

		s.ROFile = nil
	}

	if s.CwdHandle != nil {
		if err := s.CwdHandle.Close(); err != nil {
			errs = append(errs, fmt.Errorf("CwdHandle close failed: %w", err))
		}

		s.CwdHandle = nil
	}

	if s.WOFile != nil {
		if err := s.WOFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("WOFile close failed: %w", err))
		}

		s.WOFile = nil
	}

	s.CDSectorSize = 0

	return errors.Join(errs...)
}
