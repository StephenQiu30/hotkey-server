package dto

import "errors"

var (
	ReportErrNotFound   = errors.New("report not found")
	HotEventErrNotFound = errors.New("hot event not found")
)
