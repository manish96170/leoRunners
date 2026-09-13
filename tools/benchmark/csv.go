package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

func WriteCSV(w io.Writer, samples []Sample) error {
	writer := csv.NewWriter(w)
	header := []string{"id", "started_at", "completed_at"}
	for _, phase := range phases {
		header = append(header, string(phase)+"_ms")
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, sample := range SortedSamples(samples) {
		row := []string{sample.ID, sample.StartedAt.Format("2006-01-02T15:04:05.000000000Z07:00"), sample.CompletedAt.Format("2006-01-02T15:04:05.000000000Z07:00")}
		for _, phase := range phases {
			row = append(row, strconv.FormatInt(sample.Duration(phase).Milliseconds(), 10))
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write %s: %w", sample.ID, err)
		}
	}
	writer.Flush()
	return writer.Error()
}
