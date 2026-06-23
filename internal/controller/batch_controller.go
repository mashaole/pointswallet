package controller

import (
	"net/http"

	"pointswallet/internal/config"
	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	batchsvc "pointswallet/internal/service/batch"
)

type BatchController struct {
	batch *batchsvc.Service
	cfg   config.Config
}

func NewBatchController(batch *batchsvc.Service, cfg config.Config) *BatchController {
	return &BatchController{batch: batch, cfg: cfg}
}

func (c *BatchController) Upload(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	if err := r.ParseMultipartForm(c.cfg.MaxRequestBodyBytes); err != nil {
		writeError(w, models.NewAPIError("validation_error", "Invalid multipart form", http.StatusBadRequest))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, models.NewAPIError("validation_error", "file field required", http.StatusBadRequest))
		return
	}
	defer file.Close()

	rows, err := batchsvc.ParseCSV(file)
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	jobID := batchsvc.NewJobID()
	if err := c.batch.CreateJob(r.Context(), jobID, rows); err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	c.batch.ProcessAsync(r.Context(), jobID, rows, claims.Sub)
	writeData(w, http.StatusAccepted, map[string]any{
		"batch_job_id": jobID,
		"status":       models.BatchStatusQueued,
		"total_rows":   len(rows),
	})
}

func (c *BatchController) GetJob(w http.ResponseWriter, r *http.Request) {
	job, err := c.batch.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	resp := map[string]any{
		"batch_job_id": job.ID,
		"status":       job.Status,
		"total_rows":   job.TotalRows,
		"accepted":     job.AcceptedCount,
		"rejected":     job.RejectedCount,
		"duplicates":   job.DuplicateCount,
		"created_at":   job.CreatedAt,
	}
	if job.StartedAt != nil {
		resp["started_at"] = job.StartedAt
	}
	if job.CompletedAt != nil {
		resp["completed_at"] = job.CompletedAt
	}
	if job.ErrorMessage != nil {
		resp["error_message"] = *job.ErrorMessage
	}
	writeData(w, http.StatusOK, resp)
}

func (c *BatchController) Audit(w http.ResponseWriter, r *http.Request) {
	p := dto.ParsePagination(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"),
		c.cfg.PaginationDefaultLimit, c.cfg.PaginationMaxLimit)
	if err := p.Validate(c.cfg.PaginationMaxLimit); err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	events, total, err := c.batch.ListAudit(r.Context(), r.PathValue("id"), p.Limit, p.Offset)
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}
	data := make([]map[string]any, 0, len(events))
	for _, e := range events {
		data = append(data, map[string]any{
			"ref":        e.Ref,
			"account_id": e.AccountID,
			"status":     e.Status,
			"reason":     e.Reason,
			"created_at": e.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": paginationMeta(p.Limit, p.Offset, total, len(data)),
	})
}
