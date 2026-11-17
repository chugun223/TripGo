package statuscode

import "net/http"

func ToInt32(status int) int32 {
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}

	return int32(status)
}
