package api

import (
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/tfenng/tasks/task2go/internal/store"
)

func percentile(values []float64, pct float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	index := int(math.Round((pct / 100) * float64(len(ordered)-1)))
	if index < 0 {
		index = 0
	}
	if index >= len(ordered) {
		index = len(ordered) - 1
	}
	value := ordered[index]
	return &value
}

func durationMS(run store.Run) *float64 {
	if run.StartedAt == nil || run.FinishedAt == nil {
		return nil
	}
	value := math.Max(0, run.FinishedAt.Sub(*run.StartedAt).Seconds()*1000)
	return &value
}

func bucketKey(run store.Run, groupBy, tzName string, version *store.DefinitionVersion) (string, bool) {
	switch groupBy {
	case "none":
		return "all", true
	case "day":
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			return "", false
		}
		return run.CreatedAt.In(loc).Format("2006-01-02"), true
	case "definition":
		return run.DefinitionID, true
	case "trigger":
		return run.TriggerType, true
	case "executor":
		if version != nil {
			return version.ExecutorType, true
		}
		return "unknown", true
	default:
		return "", false
	}
}

func buildBucket(key string, runs []store.Run) StatsBucket {
	bucket := StatsBucket{Key: key, Total: len(runs)}
	var durations []float64
	for _, run := range runs {
		switch run.Status {
		case "succeeded":
			bucket.Succeeded++
		case "failed":
			bucket.Failed++
		case "cancelled":
			bucket.Cancelled++
		case "skipped":
			bucket.Skipped++
		case "missed":
			bucket.Missed++
		}
		if value := durationMS(run); value != nil {
			durations = append(durations, *value)
		}
	}
	denominator := bucket.Succeeded + bucket.Failed
	if denominator > 0 {
		bucket.SuccessRate = float64(bucket.Succeeded) / float64(denominator) * 100
	}
	if len(durations) > 0 {
		sum := 0.0
		for _, value := range durations {
			sum += value
		}
		avg := sum / float64(len(durations))
		bucket.AvgDurationMS = &avg
	}
	bucket.P95DurationMS = percentile(durations, 95)
	return bucket
}

func (s *Server) runStats(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	from, apiErr := parseOptionalTime(r, "from")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	to, apiErr := parseOptionalTime(r, "to")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	now := time.Now().UTC()
	if from == nil {
		v := now.AddDate(0, 0, -7)
		from = &v
	}
	if to == nil {
		to = &now
	}
	timezone := r.URL.Query().Get("timezone")
	if timezone == "" {
		timezone = "UTC"
	}
	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "none"
	}
	definitionID := parseOptionalString(r, "definition_id")
	definitionVersionID := parseOptionalString(r, "definition_version_id")
	triggerType := parseOptionalString(r, "trigger_type")
	executorType := parseOptionalString(r, "executor_type")
	ctx := r.Context()
	runs, err := s.Store.RunsInRange(ctx, s.Store.Pool, *from, *to)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	filtered := runs[:0]
	versionIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		if definitionID != nil && run.DefinitionID != *definitionID {
			continue
		}
		if definitionVersionID != nil && run.DefinitionVersionID != *definitionVersionID {
			continue
		}
		if triggerType != nil && run.TriggerType != *triggerType {
			continue
		}
		filtered = append(filtered, run)
		versionIDs = append(versionIDs, run.DefinitionVersionID)
	}
	versions, err := s.Store.VersionsByIDs(ctx, s.Store.Pool, versionIDs)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if executorType != nil {
		kept := filtered[:0]
		for _, run := range filtered {
			if version := versions[run.DefinitionVersionID]; version != nil && version.ExecutorType == *executorType {
				kept = append(kept, run)
			}
		}
		filtered = kept
	}
	grouped := map[string][]store.Run{}
	for _, run := range filtered {
		key, ok := bucketKey(run, groupBy, timezone, versions[run.DefinitionVersionID])
		if !ok {
			writeError(w, r, validationErr("invalid group_by"))
			return
		}
		grouped[key] = append(grouped[key], run)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buckets := make([]StatsBucket, 0, len(keys))
	for _, key := range keys {
		buckets = append(buckets, buildBucket(key, grouped[key]))
	}
	total := buildBucket("all", filtered)
	writeJSON(w, 200, RunStats{
		Total: total.Total, Succeeded: total.Succeeded, Failed: total.Failed,
		Cancelled: total.Cancelled, Skipped: total.Skipped, Missed: total.Missed,
		Retrying: countStatus(filtered, "retry_wait"), SuccessRate: total.SuccessRate,
		AvgDurationMS: total.AvgDurationMS, P95DurationMS: total.P95DurationMS,
		Buckets: buckets,
	})
}

func countStatus(runs []store.Run, status string) int {
	n := 0
	for _, run := range runs {
		if run.Status == status {
			n++
		}
	}
	return n
}

func (s *Server) attemptStats(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator", "viewer"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	from, apiErr := parseOptionalTime(r, "from")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	to, apiErr := parseOptionalTime(r, "to")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	now := time.Now().UTC()
	if from == nil {
		v := now.AddDate(0, 0, -7)
		from = &v
	}
	if to == nil {
		to = &now
	}
	attempts, err := s.Store.AttemptsInRange(r.Context(), s.Store.Pool, *from, *to)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	out := AttemptStats{Total: len(attempts), Buckets: []StatsBucket{}}
	var durations []float64
	for _, attempt := range attempts {
		switch attempt.Status {
		case "succeeded":
			out.Succeeded++
		case "failed":
			out.Failed++
		case "timed_out":
			out.TimedOut++
		case "cancelled":
			out.Cancelled++
		}
		if attempt.DurationMS != nil {
			durations = append(durations, float64(*attempt.DurationMS))
		}
	}
	if out.Total > 0 {
		out.SuccessRate = float64(out.Succeeded) / float64(out.Total) * 100
	}
	if len(durations) > 0 {
		sum := 0.0
		for _, value := range durations {
			sum += value
		}
		avg := sum / float64(len(durations))
		out.AvgDurationMS = &avg
	}
	writeJSON(w, 200, out)
}
