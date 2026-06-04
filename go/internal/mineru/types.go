// Package mineru implements the MinerU API client and per-file task
// processing. It mirrors the Python reference (python/mineru_api.py) and
// is composed of three small files:
//
//   - types.go    : data types and enums
//   - client.go   : HTTP client and polling/upload/download primitives
//   - process.go  : per-file workflow and concurrent dispatch
package mineru

// TaskStatus is the high-level outcome of processing a single file.
type TaskStatus int

const (
	// TaskDone means the file was successfully processed (or recovered).
	TaskDone TaskStatus = 0
	// TaskSkip means the file was already processed previously.
	TaskSkip TaskStatus = 1
	// TaskFail means processing did not complete in this run (network
	// failure, API error, or poll timeout).
	TaskFail TaskStatus = 2
)

// String returns the lowercase status name used in log output and JSON
// task state. It mirrors the Python enum values ("done", "skip", "fail").
func (s TaskStatus) String() string {
	switch s {
	case TaskDone:
		return "done"
	case TaskSkip:
		return "skip"
	case TaskFail:
		return "fail"
	default:
		return "unknown"
	}
}

// TaskInfo is the per-file state persisted to "<stem>_task.json" so
// processing can resume across runs. Field names match the Python dict
// keys for compatibility with existing on-disk state.
type TaskInfo struct {
	File         string `json:"file"`
	BatchID      string `json:"batch_id"`
	CreatedAt    string `json:"created_at"`
	Status       string `json:"status"`
	LastError    string `json:"last_error"`
	OutputFolder string `json:"output_folder"`
}

// ExtractProgress is the per-task progress sub-document returned by the
// MinerU API inside each extract_result item.
type ExtractProgress struct {
	ExtractedPages int `json:"extracted_pages"`
	TotalPages     int `json:"total_pages"`
}

// ExtractResultItem is one entry in the extract_result array returned by
// the poll endpoint. Only the fields we actually inspect are declared;
// unknown fields are tolerated by encoding/json.
type ExtractResultItem struct {
	State         string           `json:"state"`
	ErrMsg        string           `json:"err_msg"`
	FullZipURL    string           `json:"full_zip_url"`
	ExtractProgress ExtractProgress `json:"extract_progress"`
}

// ExtractResultData is the "data" sub-document of the poll response.
type ExtractResultData struct {
	ExtractResult []ExtractResultItem `json:"extract_result"`
}

// BatchResult is the JSON body returned by the poll endpoint. Code 0
// means success; non-zero codes carry an error message in Msg.
type BatchResult struct {
	Code int              `json:"code"`
	Msg  string           `json:"msg"`
	Data ExtractResultData `json:"data"`
}

// UploadTaskData is the "data" sub-document of the create-task response.
type UploadTaskData struct {
	BatchID  string   `json:"batch_id"`
	FileURLs []string `json:"file_urls"`
}

// UploadTaskResponse is the JSON body returned by POST /file-urls/batch.
type UploadTaskResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data UploadTaskData `json:"data"`
}
