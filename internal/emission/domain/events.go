package domain

import (
	"time"

	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/google/uuid"
)

const EventEmissionCalculated eventbus.EventType = "emission.calculated"

type EmissionCalculatedPayload struct {
	EmissionID uuid.UUID
	ActivityID uuid.UUID
	UserID     uuid.UUID
	KgCO2e     float64
	DateLocal  time.Time
}
