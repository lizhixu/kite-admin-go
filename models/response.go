package models

type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	OriginUrl string      `json:"originUrl"`
}

type PageData struct {
	PageData interface{} `json:"pageData"`
	Total    int64       `json:"total"`
}
