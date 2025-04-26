package kit

import "encoding/json"

func j(result any) string {
	json, err := json.Marshal(result)
	if err != nil {
		return err.Error()
	}
	return string(json)
}
