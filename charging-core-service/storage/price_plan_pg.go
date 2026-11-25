package storage

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type pricePlanRow struct {
	ID             string `gorm:"primaryKey"`
	TenantID       string `gorm:"index"`
	StationID      string `gorm:"index"`
	DeviceType     string `gorm:"index"`
	EffectiveAt    time.Time
	EffectiveNow   bool   `gorm:"column:effective_now"`
	Status         string `gorm:"index"`
	Source         string
	Operator       string
	Note           string
	ModelCode      int `gorm:"column:model_code"`
	DeliveredCount int    `gorm:"column:delivered_count"`
	SuccessCount   int    `gorm:"column:success_count"`
	CreatedAt      time.Time
}

func (pricePlanRow) TableName() string { return "price_plan" }

type priceSegmentRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	PlanID       string `gorm:"index;column:plan_id"`
	Seq          int
	StartMin     int `gorm:"column:start_min"`
	EndMin       int `gorm:"column:end_min"`
	Period       string
	ElecPrice    int64 `gorm:"column:elec_price"`
	ServicePrice int64 `gorm:"column:service_price"`
}

func (priceSegmentRow) TableName() string { return "price_plan_segment" }

type PGPricePlanStore struct{ S *PGStore }

func (w *PGPricePlanStore) Save(ctx context.Context, plan PricePlan) error {
	return w.S.db.Transaction(func(tx *gorm.DB) error {
		row := pricePlanRow{
			ID: plan.ID, TenantID: plan.TenantID, StationID: plan.StationID,
			DeviceType: plan.DeviceType, EffectiveAt: plan.EffectiveAt,
			EffectiveNow: plan.EffectiveNow, Status: plan.Status, Source: plan.Source,
			Operator: plan.Operator, Note: plan.Note, ModelCode: plan.ModelCode,
			DeliveredCount: plan.DeliveredCount, SuccessCount: plan.SuccessCount,
			CreatedAt: plan.CreatedAt,
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := tx.Where("plan_id = ?", plan.ID).Delete(&priceSegmentRow{}).Error; err != nil {
			return err
		}
		for _, seg := range plan.Segments {
			sr := priceSegmentRow{
				PlanID: plan.ID, Seq: seg.Seq, StartMin: seg.StartMin, EndMin: seg.EndMin,
				Period: string(seg.Period), ElecPrice: int64(seg.ElecPrice),
				ServicePrice: int64(seg.ServicePrice),
			}
			if err := tx.Create(&sr).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *PGPricePlanStore) Get(ctx context.Context, id string) (PricePlan, bool, error) {
	var r pricePlanRow
	err := w.S.db.Where("id = ?", id).First(&r).Error
	if err == gorm.ErrRecordNotFound {
		return PricePlan{}, false, nil
	}
	if err != nil {
		return PricePlan{}, false, err
	}
	segs, err := w.segments(ctx, id)
	if err != nil {
		return PricePlan{}, false, err
	}
	return toPlan(r, segs), true, nil
}

func (w *PGPricePlanStore) Current(ctx context.Context, tenantID, stationID, deviceType string) (PricePlan, bool, error) {
	var r pricePlanRow
	err := w.S.db.Where("tenant_id = ? AND station_id = ? AND device_type = ? AND status = ?",
		tenantID, stationID, deviceType, "valid").Order("created_at DESC").First(&r).Error
	if err == gorm.ErrRecordNotFound {
		return PricePlan{}, false, nil
	}
	if err != nil {
		return PricePlan{}, false, err
	}
	segs, err := w.segments(ctx, r.ID)
	if err != nil {
		return PricePlan{}, false, err
	}
	return toPlan(r, segs), true, nil
}

func (w *PGPricePlanStore) List(ctx context.Context, tenantID, stationID, deviceType string, history bool) ([]PricePlan, error) {
	q := w.S.db.Model(&pricePlanRow{})
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if stationID != "" {
		q = q.Where("station_id = ?", stationID)
	}
	if deviceType != "" {
		q = q.Where("device_type = ?", deviceType)
	}
	status := "valid"
	if history {
		status = "invalid"
	}
	var rows []pricePlanRow
	if err := q.Where("status = ?", status).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PricePlan, 0, len(rows))
	for _, r := range rows {
		segs, err := w.segments(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, toPlan(r, segs))
	}
	return out, nil
}

func (w *PGPricePlanStore) InvalidateCurrent(ctx context.Context, tenantID, stationID, deviceType string) error {
	return w.S.db.Model(&pricePlanRow{}).
		Where("tenant_id = ? AND station_id = ? AND device_type = ? AND status = ?",
			tenantID, stationID, deviceType, "valid").
		Update("status", "invalid").Error
}

func (w *PGPricePlanStore) segments(ctx context.Context, planID string) ([]PriceSegment, error) {
	var rows []priceSegmentRow
	if err := w.S.db.Where("plan_id = ?", planID).Order("seq").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PriceSegment, 0, len(rows))
	for _, r := range rows {
		out = append(out, PriceSegment{
			Seq: r.Seq, StartMin: r.StartMin, EndMin: r.EndMin,
			Period: Period(r.Period),
			ElecPrice: moneyPrice(r.ElecPrice), ServicePrice: moneyPrice(r.ServicePrice),
		})
	}
	return out, nil
}

var _ PricePlanStore = (*PGPricePlanStore)(nil)
var _ PricePlanStore = (*MemoryPricePlanStore)(nil)
