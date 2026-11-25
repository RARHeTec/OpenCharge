package storage

import "github.com/RARHeTec/OpenCharge/charging-common/money"

func moneyPrice(v int64) money.Price { return money.Price(v) }

func toPlan(r pricePlanRow, segs []PriceSegment) PricePlan {
	return PricePlan{
		ID: r.ID, TenantID: r.TenantID, StationID: r.StationID,
		DeviceType: r.DeviceType, EffectiveAt: r.EffectiveAt,
		EffectiveNow: r.EffectiveNow, Status: r.Status, Source: r.Source,
		Operator: r.Operator, Note: r.Note, Segments: segs,
		DeliveredCount: r.DeliveredCount, SuccessCount: r.SuccessCount,
		CreatedAt: r.CreatedAt,
	}
}
