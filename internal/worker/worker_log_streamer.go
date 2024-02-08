package worker

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/reddit/monoceros/internal/common"
	"go.uber.org/zap"
)

func logRawString(wId, key, line string, log func(msg string, fields ...zap.Field)) {
	log("",
		zap.String(common.WorkerIdLogLabel, wId),
		zap.String(key, line))
}

func logJson(wId, line string, w io.Writer) bool {
	raw := json.RawMessage(line)
	if !json.Valid(raw) {
		return false
	}

	var objmap map[string]interface{}
	err := json.Unmarshal(raw, &objmap)
	if err != nil {
		// This shouldnt happen frequently but if it does we should have stats
		// to know about it as it happens.
		failedToParseLogsCounter.Inc()
		return false
	}
	if _, ok := objmap[common.WorkerIdLogLabel]; ok {
		// This should also be rare but collisions might be tricky.
		failedToParseLogsCounter.Inc()
		return false
	}

	objmap[common.WorkerIdLogLabel] = wId
	data, err := json.Marshal(objmap)
	if err != nil {
		// This shouldnt happen frequently but if it does we should have stats
		// to know about it as it happens.
		failedToParseLogsCounter.Inc()
		return false
	}
	// In this case we want to use Fprintln rather than zap logger and the reason for that is
	// that we want to make the logging proxying transparent to the application.
	// So for example if the process is logging `{"foo":"bar"}`
	// We don't want to create a zap logged structure in the form {"severity": "info", "msg":{"foo":"bar"}, "ts": 1223}
	// This has two problems:
	// 1. We can't match the severity of the original request
	// 2. It confuses log parsing progams such as logDNA
	// In that case we want to be able to write directly to an stdstream
	fmt.Fprintf(w, "%s\n", data)
	return true
}

// We always attempt to flatten the log line if it is
// a valid json and if not we log it as nested message
func LogStdOut(wId, line string, logger *zap.Logger, w io.Writer) {
	if !logJson(wId, line, w) {
		logRawString(wId, "worker_info_msg", line, logger.Info)
	}
}

func LogStdErr(wId, line string, logger *zap.Logger, w io.Writer) {
	if !logJson(wId, line, w) {
		logRawString(wId, "worker_error_msg", line, logger.Error)
	}
}
