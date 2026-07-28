package memoryworker

import (
	"neo-chat/mm-chat/backend/internal/strictjson"
)

func strictDecodeProviderJSON(body []byte, target any) error {
	return strictjson.Decode(body, memoryExtractionOutputBytes, target)
}
