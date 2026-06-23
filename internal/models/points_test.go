package models_test

import (
	"testing"

	"pointswallet/internal/models"
)

func TestPointsFromWhole(t *testing.T) {
	p := models.PointsFromWhole(150)
	if p.Int64() != 15000 {
		t.Fatalf("expected 15000 point-cents, got %d", p.Int64())
	}
	if p.WholePoints() != 150 {
		t.Fatalf("expected 150 whole points, got %d", p.WholePoints())
	}
}

func TestPointsArithmetic(t *testing.T) {
	balance := models.PointsFromWhole(100)
	spend := models.PointsFromWhole(30)
	after := balance.Sub(spend)
	if after.WholePoints() != 70 {
		t.Fatalf("expected 70, got %d", after.WholePoints())
	}
}
