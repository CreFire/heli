package xhttp

import (
	"encoding/json"
	"game/src/proto/errorpb"
)

var ErrorHttpJsonUnMarshalFailed = NewHttpError(int32(errorpb.ERROR_UNMARSHAL_JSON_FAILED), "unmarshal json failed")
var ErrorHttpOctetUnMarshalFailed = NewHttpError(int32(errorpb.ERROR_UNMARSHAL_OCTET_FAILED), "marshal octet failed")
var ErrorHttpUnSupportMethod = NewHttpError(int32(errorpb.ERROR_UNSUPPORT_HTTP_METHOD), "un support http method")
var ErrorHttpUnSupportContentType = NewHttpError(int32(errorpb.ERROR_UNSUPPORT_HTTP_CONTENT_TYPE), "un support http content type")
var ErrorHttpUnexpect = NewHttpError(int32(errorpb.ERROR_UNEXPECTED), "unexpect error")
var ErrorHttpBodyTooLarge = NewHttpError(int32(errorpb.ERROR_UNEXPECTED), "request body too large")

type HttpError struct {
	ErrCode int32             `json:"err_code"`
	ErrDesc string            `json:"err_desc"`
	Meta    map[string]string `json:"meta,omitempty"`
}

func NewHttpError(errCode int32, errDesc string) *HttpError {
	return &HttpError{ErrCode: errCode, ErrDesc: errDesc}
}

func NewHttpErrorWithMeta(errCode int32, errDesc string, meta map[string]string) *HttpError {
	return &HttpError{ErrCode: errCode, ErrDesc: errDesc, Meta: meta}
}

func (e *HttpError) Error() string {
	m := map[string]any{
		"err_code": e.ErrCode,
		"err_desc": e.ErrDesc,
		"meta":     e.Meta,
	}
	js, _ := json.Marshal(m)
	return string(js)
}

func (e *HttpError) GetErrCode() int32 {
	return e.ErrCode
}

func (e *HttpError) GetErrDesc() string {
	return e.ErrDesc
}

func (e *HttpError) GetMeta() map[string]string {
	return e.Meta
}

func NewWithError(code int32, err error) *HttpError {
	return &HttpError{ErrCode: code, ErrDesc: err.Error()}
}

func (s *HttpError) WithMeta(meta map[string]string) *HttpError {
	s.Meta = meta
	return s
}

func (s *HttpError) ProtoHttpError() *errorpb.HttpError {
	return &errorpb.HttpError{
		ErrCode:  s.ErrCode,
		ErrDesc:  s.ErrDesc,
		MetaData: s.Meta,
	}
}
