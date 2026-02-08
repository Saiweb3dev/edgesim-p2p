package sensor

import "encoding/json"

// EncodeReading serializes a reading for transport or storage.
func EncodeReading(reading Reading) ([]byte, error) {
	return json.Marshal(reading)
}

// DecodeReading parses a serialized reading.
func DecodeReading(payload []byte) (Reading, error) {
	var reading Reading
	err := json.Unmarshal(payload, &reading)
	return reading, err
}
