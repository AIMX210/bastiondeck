package httpx

import (
	"net/http"
	"strings"
	"time"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/jobs"
)

func execReq(cmd string, timeoutSec int) connector.ExecRequest {
	return connector.ExecRequest{Command: cmd, Timeout: time.Duration(timeoutSec) * time.Second}
}

// resolveTargets expands group ids / "all" into concrete host ids.
func (s *Server) resolveTargets(r *http.Request, ids []string, groupID string) ([]string, error) {
	out := append([]string{}, ids...)
	if groupID != "" {
		hosts, err := s.deps.Hosts.List(r.Context(), filterByGroup(groupID))
		if err != nil {
			return nil, err
		}
		for _, h := range hosts {
			out = append(out, h.ID)
		}
	}
	return dedup(out), nil
}

func (s *Server) execOnce(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command     string            `json:"command"`
		SnippetID   string            `json:"snippetId"`
		Vars        map[string]string `json:"vars"`
		TargetIDs   []string          `json:"targetIds"`
		GroupID     string            `json:"groupId"`
		TimeoutSec  int               `json:"timeoutSec"`
		Concurrency int               `json:"concurrency"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	cmd := req.Command
	if req.SnippetID != "" {
		sn, err := s.deps.Snippets.Get(r.Context(), req.SnippetID)
		if err != nil {
			writeErr(w, 404, "not_found", err.Error())
			return
		}
		cmd = sn.Body
	}
	// Render ${var} placeholders for BOTH snippet bodies and ad-hoc commands;
	// block on any unfilled variable instead of shipping a literal "${name}".
	rendered, missing := renderBody(cmd, req.Vars)
	if len(missing) > 0 {
		writeErr(w, 422, "missing_vars", "missing variables: "+strings.Join(missing, ", "))
		return
	}
	cmd = rendered
	targets, err := s.resolveTargets(r, req.TargetIDs, req.GroupID)
	if err != nil {
		fail(w, err)
		return
	}
	u, _ := CurrentUser(r)
	runID, err := s.deps.Jobs.StartRun(r.Context(), jobs.StartInput{
		JobName: "adhoc", Command: cmd, TargetIDs: targets,
		Timeout:     time.Duration(orDefault(req.TimeoutSec, 60)) * time.Second,
		Concurrency: orDefault(req.Concurrency, 5), Trigger: "manual",
		Actor: s.actorOf(r),
	})
	_ = u
	if err != nil {
		switch err {
		case jobs.ErrEmptyTargets:
			writeErr(w, 422, "empty_targets", err.Error())
		case jobs.ErrEmptyCommand:
			writeErr(w, 422, "command_empty", err.Error())
		default:
			fail(w, err)
		}
		return
	}
	writeJSON(w, 202, map[string]any{"runId": runID})
}

func (s *Server) runJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID string `json:"jobId"`
	}
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	job, err := s.deps.JobRepo.GetJob(r.Context(), req.JobID)
	if err != nil {
		writeErr(w, 404, "not_found", err.Error())
		return
	}
	runID, err := s.deps.Jobs.StartRun(r.Context(), jobs.StartInput{
		JobID: job.ID, JobName: job.Name, Command: job.Command, TargetIDs: job.TargetIDs,
		Timeout: time.Duration(job.TimeoutMs) * time.Millisecond, Concurrency: job.Concurrency,
		Trigger: "manual", Actor: s.actorOf(r)})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"runId": runID})
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jb, err := s.deps.JobRepo.ListJobs(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"jobs": jb})
}

type jobReq struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Command      string   `json:"command"`
	TargetIDs    []string `json:"targetIds"`
	GroupID      string   `json:"groupId"`
	ScheduleExpr string   `json:"scheduleExpr"`
	Enabled      *bool    `json:"enabled"`
	TimeoutMs    int      `json:"timeoutMs"`
	Concurrency  int      `json:"concurrency"`
}

func (s *Server) toJobInput(r *http.Request, req jobReq) (jobs.JobInput, error) {
	targets, err := s.resolveTargets(r, req.TargetIDs, req.GroupID)
	if err != nil {
		return jobs.JobInput{}, err
	}
	kind := req.Kind
	if kind == "" {
		if req.ScheduleExpr != "" {
			kind = "scheduled"
		} else {
			kind = "adhoc"
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	u, _ := CurrentUser(r)
	return jobs.JobInput{
		Name: req.Name, Command: req.Command, TargetIDs: targets,
		ScheduleExpr: req.ScheduleExpr, Enabled: enabled,
		TimeoutMs: orDefault(req.TimeoutMs, 60000), Concurrency: orDefault(req.Concurrency, 5),
		Kind: kind, CreatedBy: u.ID,
	}, nil
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req jobReq
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	in, err := s.toJobInput(r, req)
	if err != nil {
		fail(w, err)
		return
	}
	j, err := s.deps.JobRepo.CreateJob(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	if j.ScheduleExpr != "" && s.deps.Scheduler != nil {
		_ = s.deps.Scheduler.RecomputeNext(r.Context(), j.ID, j.ScheduleExpr, time.Now())
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "job.create", "job", j.ID, "success", nil)
	writeJSON(w, 201, map[string]any{"job": j})
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req jobReq
	if err := decodeJSON(r, &req, 0); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	in, err := s.toJobInput(r, req)
	if err != nil {
		fail(w, err)
		return
	}
	j, err := s.deps.JobRepo.UpdateJob(r.Context(), id, in)
	if err != nil {
		fail(w, err)
		return
	}
	if j.ScheduleExpr != "" && s.deps.Scheduler != nil {
		_ = s.deps.Scheduler.RecomputeNext(r.Context(), j.ID, j.ScheduleExpr, time.Now())
	}
	writeJSON(w, 200, map[string]any{"job": j})
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.JobRepo.DeleteJob(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	limit := clamp(atoiDefault(r.URL.Query().Get("limit"), 50), 1, 100)
	runs, next, err := s.deps.JobRepo.ListRuns(r.Context(), limit, cursorOf(r.URL.Query().Get("cursor")))
	if err != nil {
		fail(w, err)
		return
	}
	resp := map[string]any{"runs": runs}
	if next > 0 {
		resp["nextCursor"] = next
	}
	writeJSON(w, 200, resp)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.deps.JobRepo.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "not_found", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"run": run, "live": s.deps.Jobs.IsLive(run.ID)})
}

func (s *Server) runOutput(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	tid := r.PathValue("tid")
	stream := r.URL.Query().Get("stream")
	if stream != "stderr" {
		stream = "stdout"
	}
	off := cursorOf(r.URL.Query().Get("offset"))
	b, newOff, err := s.deps.Jobs.ReadOutput(r.Context(), runID, tid, stream, off)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"chunk": string(b), "offset": newOff})
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Jobs.CancelRun(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_, _ = s.deps.Audit.Write(r.Context(), s.actorOf(r), "job.cancel", "job_run", id, "success", nil)
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newID, err := s.deps.Jobs.RetryFailed(r.Context(), id, s.actorOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"runId": newID})
}
