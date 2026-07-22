package trackingmodel

import "testing"

func TestExpressionMaskSetHasAndOutOfRange(t *testing.T) {
	var mask ExpressionMask
	if !mask.Set(ExpressionJawOpen) {
		t.Fatal("Set(ExpressionJawOpen) = false")
	}
	if !mask.Has(ExpressionJawOpen) {
		t.Fatal("Has(ExpressionJawOpen) = false")
	}
	if mask.Set(ExpressionCount) {
		t.Fatal("Set(ExpressionCount) = true")
	}
	if mask.Has(ExpressionCount) {
		t.Fatal("Has(ExpressionCount) = true")
	}
	if !ExpressionMaskOf(ExpressionJawOpen, ExpressionCount).Has(ExpressionJawOpen) {
		t.Fatal("ExpressionMaskOf did not retain valid ID")
	}
}

func TestExpressionMaskIsZero(t *testing.T) {
	var mask ExpressionMask
	if !mask.IsZero() {
		t.Fatal("zero mask IsZero() = false")
	}
	mask.Set(ExpressionJawOpen)
	if mask.IsZero() {
		t.Fatal("nonzero mask IsZero() = true")
	}
}

func TestExpressionMaskIntersect(t *testing.T) {
	left := ExpressionMaskOf(ExpressionJawOpen, ExpressionTongueOut)
	right := ExpressionMaskOf(ExpressionJawOpen, ExpressionBrowPinchRight)

	intersection := left.Intersect(right)
	if !intersection.Has(ExpressionJawOpen) {
		t.Fatal("intersection omitted shared ID")
	}
	if intersection.Has(ExpressionTongueOut) || intersection.Has(ExpressionBrowPinchRight) {
		t.Fatal("intersection retained non-shared ID")
	}
}

func TestExpressionMaskNormalizeClearsUnusedHighBits(t *testing.T) {
	mask := ExpressionMask{Words: [2]uint64{^uint64(0), ^uint64(0)}}
	normalized := mask.Normalize()

	if normalized.Words[0] != ^uint64(0) {
		t.Fatalf("first word = %#x, want unchanged", normalized.Words[0])
	}
	wantLastWord := uint64(1)<<(ExpressionCount%64) - 1
	if normalized.Words[1] != wantLastWord {
		t.Fatalf("last word = %#x, want %#x", normalized.Words[1], wantLastWord)
	}
}

func TestExpressionSetSetGetClearAndOutOfRange(t *testing.T) {
	var set ExpressionSet
	if !set.Set(ExpressionJawOpen, .75) {
		t.Fatal("Set(ExpressionJawOpen) = false")
	}
	if got, ok := set.Get(ExpressionJawOpen); !ok || got != .75 {
		t.Fatalf("Get(ExpressionJawOpen) = %v, %v, want .75, true", got, ok)
	}
	if !set.Clear(ExpressionJawOpen) {
		t.Fatal("Clear(ExpressionJawOpen) = false")
	}
	if got, ok := set.Get(ExpressionJawOpen); ok || got != 0 {
		t.Fatalf("Get(ExpressionJawOpen) after Clear = %v, %v, want 0, false", got, ok)
	}
	if set.Set(ExpressionCount, 1) {
		t.Fatal("Set(ExpressionCount) = true")
	}
	if got, ok := set.Get(ExpressionCount); ok || got != 0 {
		t.Fatalf("Get(ExpressionCount) = %v, %v, want 0, false", got, ok)
	}
	if set.Clear(ExpressionCount) {
		t.Fatal("Clear(ExpressionCount) = true")
	}
}

func TestExpressionNamesAreCompleteUniqueAndCopied(t *testing.T) {
	names := ExpressionNames()
	if len(names) != int(ExpressionCount) {
		t.Fatalf("len(ExpressionNames()) = %d, want %d", len(names), ExpressionCount)
	}

	seen := make(map[string]ExpressionID, len(names))
	for id, name := range names {
		if name == "" {
			t.Errorf("ExpressionNames()[%d] is empty", id)
		}
		if previous, duplicate := seen[name]; duplicate {
			t.Errorf("ExpressionNames()[%d] = %q duplicates ID %d", id, name, previous)
		}
		seen[name] = ExpressionID(id)
	}

	original := names[ExpressionJawOpen]
	names[ExpressionJawOpen] = "mutated"
	if got := ExpressionNames()[ExpressionJawOpen]; got != original {
		t.Fatalf("ExpressionNames() shared package storage: got %q, want %q", got, original)
	}
}

func TestExpressionIDsRemainStableAcrossGroups(t *testing.T) {
	tests := []struct {
		id   ExpressionID
		want ExpressionID
	}{
		{ExpressionEyeSquintRight, 0},
		{ExpressionBrowPinchRight, 2},
		{ExpressionNoseSneerRight, 10},
		{ExpressionCheekSquintRight, 16},
		{ExpressionJawOpen, 20},
		{ExpressionLipSuckUpperRight, 26},
		{ExpressionLipFunnelUpperRight, 32},
		{ExpressionLipPuckerUpperRight, 36},
		{ExpressionMouthUpperUpRight, 40},
		{ExpressionTongueOut, 64},
	}
	for _, test := range tests {
		if test.id != test.want {
			t.Errorf("ExpressionID = %d, want %d", test.id, test.want)
		}
	}
	if ExpressionCount != 76 {
		t.Fatalf("ExpressionCount = %d, want 76", ExpressionCount)
	}
}
