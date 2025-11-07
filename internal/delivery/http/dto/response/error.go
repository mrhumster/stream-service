package response

type Error struct {
	Error string `json:"error"`
}

func ErrorResponse(text string) Error {
	return Error{
		Error: text,
	}
}
