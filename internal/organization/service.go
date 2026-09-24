package organization

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Options struct {
	Clock       func() time.Time
	IDGenerator func() string
	AfterPlan   func() error
}

type Service struct {
	now       func() time.Time
	newID     func() string
	afterPlan func() error
}

func NewService(options Options) (*Service, error) {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.IDGenerator == nil {
		options.IDGenerator = uuid.NewString
	}
	if options.Clock == nil {
		return nil, errors.New("organization clock is required")
	}
	if options.IDGenerator == nil {
		return nil, errors.New("organization id generator is required")
	}

	return &Service{
		now:       options.Clock,
		newID:     options.IDGenerator,
		afterPlan: options.AfterPlan,
	}, nil
}
