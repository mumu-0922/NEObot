package rageval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func DecodeGoldenSet(reader io.Reader) (GoldenSet, error) {
	var value GoldenSet
	if err := decodeOne(reader, &value); err != nil {
		return GoldenSet{}, fmt.Errorf("decode golden set: %w", err)
	}
	if err := validateGoldenSet(value); err != nil {
		return GoldenSet{}, err
	}
	return value, nil
}

func DecodeObservationSet(reader io.Reader) (ObservationSet, error) {
	var value ObservationSet
	if err := decodeOne(reader, &value); err != nil {
		return ObservationSet{}, fmt.Errorf("decode observation set: %w", err)
	}
	if err := validateObservationSet(value); err != nil {
		return ObservationSet{}, err
	}
	return value, nil
}

func decodeOne(reader io.Reader, target any) error {
	if reader == nil {
		return errors.New("input is required")
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
