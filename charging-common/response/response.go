package response

type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data,omitempty"`
	TraceID string `json:"traceId,omitempty"`
}

func OK[T any](data T, traceID string) Response[T] {
	return Response[T]{Code: 0, Message: "ok", Data: data, TraceID: traceID}
}

func Fail[T any](code int, message, traceID string) Response[T] {
	return Response[T]{Code: code, Message: message, TraceID: traceID}
}
