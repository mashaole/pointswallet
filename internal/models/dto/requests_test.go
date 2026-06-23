package dto

import (
	"errors"
	"testing"

	"pointswallet/internal/models"
)

func TestResolveTransactionRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		body    string
		want    string
		wantErr bool
	}{
		{name: "header only", header: "key-1", body: "", want: "key-1"},
		{name: "body only", header: "", body: "key-2", want: "key-2"},
		{name: "both match", header: " key-3 ", body: "key-3", want: "key-3"},
		{name: "header wins when body empty", header: "hdr", body: "", want: "hdr"},
		{name: "both missing", wantErr: true},
		{name: "both differ", header: "a", body: "b", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTransactionRef(tt.header, tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTransactionRef_missingIsValidation(t *testing.T) {
	t.Parallel()
	_, err := ResolveTransactionRef("", "")
	if !errors.Is(err, models.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
