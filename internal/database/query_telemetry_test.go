package database

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestQueryTelemetryIsBoundedAndReportsErrors(t *testing.T) {
	label := fmt.Sprintf("test.%d", time.Now().UnixNano())
	for index := 0; index < querySampleLimit+50; index++ {
		var err error
		if index == 0 {
			err = errors.New("synthetic")
		}
		ObserveQuery(label, time.Duration(index+1)*time.Microsecond, err)
	}
	snapshot := QueryTelemetrySnapshot()[label]
	if snapshot.Count != querySampleLimit+50 || snapshot.Errors != 1 || snapshot.P95MS <= 0 || snapshot.MaxMS <= 0 {
		t.Fatalf("unexpected query telemetry: %+v", snapshot)
	}
}
