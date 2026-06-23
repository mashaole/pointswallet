package models

const PointsScale int64 = 100

// Points stores point-cents (fixed scale: 100 = 1.00 whole point).
type Points int64

func PointsFromWhole(whole int64) Points {
	return Points(whole * PointsScale)
}

func (p Points) WholePoints() int64 {
	return int64(p) / PointsScale
}

func (p Points) Int64() int64 {
	return int64(p)
}

func (p Points) Add(delta Points) Points {
	return p + delta
}

func (p Points) Sub(delta Points) Points {
	return p - delta
}
