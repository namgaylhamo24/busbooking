package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// errorBody is the JSON shape of every error response.
type errorBody struct {
	Error    string `json:"error"`
	GRPCCode string `json:"grpc_code,omitempty"`
}

// grpcToHTTP maps gRPC status codes to HTTP status codes.
// Any code not listed here becomes 500.
var grpcToHTTP = map[codes.Code]int{
	codes.InvalidArgument:  http.StatusBadRequest,         // 400
	codes.NotFound:         http.StatusNotFound,           // 404
	codes.AlreadyExists:    http.StatusConflict,           // 409
	codes.PermissionDenied: http.StatusForbidden,          // 403
	codes.Unauthenticated:  http.StatusUnauthorized,       // 401
	codes.Unavailable:      http.StatusServiceUnavailable, // 503
	codes.DeadlineExceeded: http.StatusGatewayTimeout,     // 504
	codes.Unimplemented:    http.StatusNotImplemented,     // 501
}

// protoJSON writes snake_case field names (matching the .proto files),
// includes zero values, and pretty-prints for readable screenshots.
var protoJSON = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: true,
	Multiline:       true,
	Indent:          "  ",
}

func writeRaw(w http.ResponseWriter, httpStatus int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(v)
}

// writeError sends a gateway-originated error (e.g. malformed JSON).
func writeError(w http.ResponseWriter, httpStatus int, msg string) {
	writeRaw(w, httpStatus, errorBody{Error: msg})
}

// writeProto sends a protobuf message as JSON.
func writeProto(w http.ResponseWriter, httpStatus int, msg proto.Message) {
	data, err := protoJSON.Marshal(msg)
	if err != nil {
		log.Printf("encode response: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	w.Write(append(data, '\n'))
}

// writeGRPCError converts a gRPC error from a backend into an HTTP response.
func writeGRPCError(w http.ResponseWriter, err error) {
	st, _ := status.FromError(err)
	httpStatus, ok := grpcToHTTP[st.Code()]
	if !ok {
		httpStatus = http.StatusInternalServerError
	}

	msg := st.Message()
	switch {
	case httpStatus == http.StatusInternalServerError:
		log.Printf("upstream error: %v", err) // keep details in the log only
		msg = "internal server error"
	case httpStatus == http.StatusGatewayTimeout:
		msg = "upstream service timed out"
	case st.Code() == codes.Unavailable && strings.HasPrefix(msg, "connection error"):
		// Raw dial error from the gateway's own connection: don't leak addresses.
		msg = "upstream service is unavailable"
	}
	writeRaw(w, httpStatus, errorBody{Error: msg, GRPCCode: st.Code().String()})
}

// decodeBody reads the JSON request body into msg. It writes a 400 (or 413)
// response and returns false if the body is missing, too large or malformed.
// Unknown fields are rejected so typos in field names don't pass silently.
func decodeBody(w http.ResponseWriter, r *http.Request, msg proto.Message) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "could not read request body")
		}
		return false
	}
	if len(bytes.TrimSpace(data)) == 0 {
		writeError(w, http.StatusBadRequest, "request body is required")
		return false
	}
	if err := protojson.Unmarshal(data, msg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
