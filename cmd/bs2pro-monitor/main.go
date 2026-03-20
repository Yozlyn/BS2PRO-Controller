package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const reportURL = "http://127.0.0.1:20026/api/process-switch/foreground"

type foregroundReport struct {
	ProcessName string `json:"processName"`
	ReportedAt  string `json:"reportedAt"`
}

func main() {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		processName := getForegroundProcessName()
		report := foregroundReport{
			ProcessName: processName,
			ReportedAt:  time.Now().Format(time.RFC3339),
		}
		if err := postForegroundProcess(client, report); err != nil {
			log.Printf("report foreground process failed: %v", err)
		}
		<-ticker.C
	}
}

func postForegroundProcess(client *http.Client, report foregroundReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, reportURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &httpStatusError{StatusCode: resp.StatusCode}
	}
	return nil
}

type httpStatusError struct {
	StatusCode int
}

func (e *httpStatusError) Error() string {
	return http.StatusText(e.StatusCode)
}
