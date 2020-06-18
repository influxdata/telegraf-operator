package main

import (
	"os"
	"testing"

	logrTesting "github.com/go-logr/logr/testing"
)

const (
	sampleClassData = `
	[[outputs.file]]
		files = ["stdout"]
	`
)

func Test_classDataHandler_getData(t *testing.T) {
	tests := []struct {
		name       string
		classes    map[string]string
		secretName string
		className  string
		namespace  string

		want    string
		wantErr bool
	}{
		{
			name:      "data does not contain class name",
			className: "unknown",
			classes:   map[string]string{testTelegrafClass: sampleClassData},
			wantErr:   true,
		},
		{
			name:      "returns secret data",
			className: testTelegrafClass,
			classes:   map[string]string{testTelegrafClass: sampleClassData},
			want:      sampleClassData,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			logger := &logrTesting.TestLogger{T: t}

			dir := createTempClassesDirectory(t, tt.classes)
			defer os.RemoveAll(dir)

			testClassDataHandler := &classDataHandler{
				Logger:                   logger,
				TelegrafClassesDirectory: dir,
			}

			got, err := testClassDataHandler.getData(tt.className)
			if (err != nil) != tt.wantErr {
				t.Errorf("classDataHandler.getData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("classDataHandler.getData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_classDataHandler_validateClassData(t *testing.T) {
	tests := []struct {
		name    string
		classes map[string]string
		wantErr bool
	}{
		{
			name:    "returns error when no classes found",
			classes: map[string]string{},
			wantErr: true,
		},
		{
			name:    "returns no error when all elmeents are valid",
			classes: map[string]string{testTelegrafClass: sampleClassData},
			wantErr: false,
		},
		{
			name: "returns error when at least one TOML parsing error found",
			classes: map[string]string{testTelegrafClass: sampleClassData, "invalid": `
[invalid]
"invalid" = invalid
`},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &logrTesting.TestLogger{T: t}

			dir := createTempClassesDirectory(t, tt.classes)
			defer os.RemoveAll(dir)

			testClassDataHandler := &classDataHandler{
				Logger:                   logger,
				TelegrafClassesDirectory: dir,
			}

			err := testClassDataHandler.validateClassData()
			if (err != nil) != tt.wantErr {
				t.Errorf("classDataHandler.validateClassData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
