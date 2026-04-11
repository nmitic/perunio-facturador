package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// DespatchListFilter narrows ListDespatches. Empty fields are ignored.
type DespatchListFilter struct {
	DocType string
	Status  string
	Page    int
	Limit   int
}

// DespatchListResult is a paginated slice of despatches plus the total.
type DespatchListResult struct {
	Despatches []model.Despatch `json:"despatches"`
	Total      int              `json:"total"`
}

// despatchColumns lists every column read by scanDespatch. Kept in sync
// with the despatches table definition.
const despatchColumns = `
	id, tenant_id, company_id, series_id, doc_type, series, correlative, status,
	issue_date, issue_time,
	recipient_doc_type, recipient_doc_number, recipient_name, recipient_address,
	transport_modality, transfer_reason, transfer_reason_desc, start_date,
	total_weight_kg, weight_unit_code, total_packages,
	start_ubigeo, start_address, arrival_ubigeo, arrival_address,
	driver_doc_type, driver_doc_number, driver_license, driver_name,
	vehicle_plate,
	carrier_ruc, carrier_name, carrier_mtc,
	event_code, original_gre_id,
	related_doc_type, related_doc_series, related_doc_number,
	sunat_ticket, sunat_response_code, sunat_response_description,
	r2_xml_key, r2_signed_xml_key, r2_zip_key, r2_cdr_key,
	sent_at, accepted_at, created_at, updated_at
`

func scanDespatch(row pgx.Row, d *model.Despatch) error {
	return row.Scan(
		&d.ID, &d.TenantID, &d.CompanyID, &d.SeriesID, &d.DocType, &d.Series, &d.Correlative, &d.Status,
		&d.IssueDate, &d.IssueTime,
		&d.RecipientDocType, &d.RecipientDocNumber, &d.RecipientName, &d.RecipientAddress,
		&d.TransportModality, &d.TransferReason, &d.TransferReasonDesc, &d.StartDate,
		&d.TotalWeightKg, &d.WeightUnitCode, &d.TotalPackages,
		&d.StartUbigeo, &d.StartAddress, &d.ArrivalUbigeo, &d.ArrivalAddress,
		&d.DriverDocType, &d.DriverDocNumber, &d.DriverLicense, &d.DriverName,
		&d.VehiclePlate,
		&d.CarrierRUC, &d.CarrierName, &d.CarrierMTC,
		&d.EventCode, &d.OriginalGreID,
		&d.RelatedDocType, &d.RelatedDocSeries, &d.RelatedDocNumber,
		&d.SunatTicket, &d.SunatResponseCode, &d.SunatResponseDescription,
		&d.R2XmlKey, &d.R2SignedXmlKey, &d.R2ZipKey, &d.R2CdrKey,
		&d.SentAt, &d.AcceptedAt, &d.CreatedAt, &d.UpdatedAt,
	)
}

// ListDespatches returns a paginated slice of despatches for the company.
func (p *Pool) ListDespatches(ctx context.Context, companyID string, filter DespatchListFilter) (DespatchListResult, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	args := []any{companyID}
	conditions := []string{"company_id = $1"}
	if filter.DocType != "" {
		args = append(args, filter.DocType)
		conditions = append(conditions, fmt.Sprintf("doc_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	where := strings.Join(conditions, " AND ")

	var result DespatchListResult
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		countSQL := "SELECT count(*)::int FROM despatches WHERE " + where
		if err := tx.QueryRow(ctx, countSQL, args...).Scan(&result.Total); err != nil {
			return err
		}

		listArgs := append([]any{}, args...)
		listArgs = append(listArgs, limit, offset)
		listSQL := "SELECT " + despatchColumns +
			" FROM despatches WHERE " + where +
			fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

		rows, err := tx.Query(ctx, listSQL, listArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d model.Despatch
			if err := scanDespatch(rows, &d); err != nil {
				return err
			}
			result.Despatches = append(result.Despatches, d)
		}
		return rows.Err()
	})
	return result, err
}

// GetDespatch returns a single despatch scoped to the company. Nil when not found.
func (p *Pool) GetDespatch(ctx context.Context, companyID, despatchID string) (*model.Despatch, error) {
	var out *model.Despatch
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+despatchColumns+
				" FROM despatches WHERE company_id = $1 AND id = $2 LIMIT 1",
			companyID, despatchID,
		)
		var got model.Despatch
		if err := scanDespatch(row, &got); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		out = &got
		return nil
	})
	return out, err
}

// GetDespatchLines returns the goods lines for a despatch, ordered by
// line_number.
func (p *Pool) GetDespatchLines(ctx context.Context, despatchID string) ([]model.DespatchLine, error) {
	var out []model.DespatchLine
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, despatch_id, line_number, description, quantity, unit_code,
			       product_code, created_at
			FROM despatch_lines
			WHERE despatch_id = $1
			ORDER BY line_number
		`, despatchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l model.DespatchLine
			if err := rows.Scan(&l.ID, &l.DespatchID, &l.LineNumber, &l.Description,
				&l.Quantity, &l.UnitCode, &l.ProductCode, &l.CreatedAt); err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, err
}

// DespatchCreateInput is what the HTTP handler passes to CreateDespatch.
// It owns every non-derived column; server-side fields (id, status,
// timestamps, pipeline state) are not settable by the caller.
type DespatchCreateInput struct {
	CompanyID string
	SeriesID  string

	DocType     string
	Series      string
	Correlative int

	IssueDate string // YYYY-MM-DD
	IssueTime *string

	RecipientDocType   string
	RecipientDocNumber string
	RecipientName      string
	RecipientAddress   *string

	TransportModality  string
	TransferReason     string
	TransferReasonDesc *string
	StartDate          *string // YYYY-MM-DD

	TotalWeightKg  string
	WeightUnitCode string
	TotalPackages  *int

	StartUbigeo    string
	StartAddress   string
	ArrivalUbigeo  string
	ArrivalAddress string

	DriverDocType   *string
	DriverDocNumber *string
	DriverLicense   *string
	DriverName      *string
	VehiclePlate    *string

	CarrierRUC  *string
	CarrierName *string
	CarrierMTC  *string

	EventCode     *string
	OriginalGreID *string

	RelatedDocType   *string
	RelatedDocSeries *string
	RelatedDocNumber *string

	Lines []DespatchLineInput
}

// DespatchLineInput is one goods line for creating or updating a despatch.
type DespatchLineInput struct {
	LineNumber  int
	Description string
	Quantity    string
	UnitCode    string
	ProductCode *string
}

// CreateDespatch inserts a draft despatch + its lines in a single tenant
// transaction and returns the refreshed row (without lines; callers
// should call GetDespatchLines if they need them).
func (p *Pool) CreateDespatch(ctx context.Context, in DespatchCreateInput) (*model.Despatch, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	var out model.Despatch
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO despatches (
				tenant_id, company_id, series_id, doc_type, series, correlative, status,
				issue_date, issue_time,
				recipient_doc_type, recipient_doc_number, recipient_name, recipient_address,
				transport_modality, transfer_reason, transfer_reason_desc, start_date,
				total_weight_kg, weight_unit_code, total_packages,
				start_ubigeo, start_address, arrival_ubigeo, arrival_address,
				driver_doc_type, driver_doc_number, driver_license, driver_name,
				vehicle_plate,
				carrier_ruc, carrier_name, carrier_mtc,
				event_code, original_gre_id,
				related_doc_type, related_doc_series, related_doc_number
			) VALUES (
				$1,$2,$3,$4,$5,$6,'draft',
				$7,$8,
				$9,$10,$11,$12,
				$13,$14,$15,$16,
				$17,$18,$19,
				$20,$21,$22,$23,
				$24,$25,$26,$27,
				$28,
				$29,$30,$31,
				$32,$33,
				$34,$35,$36
			) RETURNING `+despatchColumns,
			tenantID, in.CompanyID, in.SeriesID, in.DocType, in.Series, in.Correlative,
			in.IssueDate, in.IssueTime,
			in.RecipientDocType, in.RecipientDocNumber, in.RecipientName, in.RecipientAddress,
			in.TransportModality, in.TransferReason, in.TransferReasonDesc, in.StartDate,
			in.TotalWeightKg, in.WeightUnitCode, in.TotalPackages,
			in.StartUbigeo, in.StartAddress, in.ArrivalUbigeo, in.ArrivalAddress,
			in.DriverDocType, in.DriverDocNumber, in.DriverLicense, in.DriverName,
			in.VehiclePlate,
			in.CarrierRUC, in.CarrierName, in.CarrierMTC,
			in.EventCode, in.OriginalGreID,
			in.RelatedDocType, in.RelatedDocSeries, in.RelatedDocNumber,
		)
		if err := scanDespatch(row, &out); err != nil {
			return err
		}

		if len(in.Lines) == 0 {
			return nil
		}
		linkRows := make([][]any, 0, len(in.Lines))
		for _, l := range in.Lines {
			linkRows = append(linkRows, []any{
				out.ID, l.LineNumber, l.Description, l.Quantity, l.UnitCode, l.ProductCode,
			})
		}
		_, err := tx.CopyFrom(ctx,
			pgx.Identifier{"despatch_lines"},
			[]string{"despatch_id", "line_number", "description", "quantity", "unit_code", "product_code"},
			pgx.CopyFromRows(linkRows),
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDespatch deletes a draft despatch. Refuses to touch rows that
// have been sent to SUNAT.
func (p *Pool) DeleteDespatch(ctx context.Context, companyID, despatchID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM despatches WHERE company_id = $1 AND id = $2 AND status = 'draft'`,
			companyID, despatchID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrDespatchNotDeletable
		}
		return nil
	})
}

// DespatchUpdateInput carries the mutable fields for a draft despatch
// update. Every field is optional — nil leaves the column unchanged.
// Lines replace the existing set when non-nil (nil = keep existing,
// empty slice = clear).
type DespatchUpdateInput struct {
	IssueDate *string
	IssueTime *string

	RecipientDocType   *string
	RecipientDocNumber *string
	RecipientName      *string
	RecipientAddress   *string

	TransportModality  *string
	TransferReason     *string
	TransferReasonDesc *string
	StartDate          *string

	TotalWeightKg  *string
	WeightUnitCode *string
	TotalPackages  *int

	StartUbigeo    *string
	StartAddress   *string
	ArrivalUbigeo  *string
	ArrivalAddress *string

	DriverDocType   *string
	DriverDocNumber *string
	DriverLicense   *string
	DriverName      *string
	VehiclePlate    *string

	CarrierRUC  *string
	CarrierName *string
	CarrierMTC  *string

	EventCode     *string
	OriginalGreID *string

	RelatedDocType   *string
	RelatedDocSeries *string
	RelatedDocNumber *string

	Lines []DespatchLineInput
}

// UpdateDraftDespatch patches a draft despatch and (optionally) replaces
// its lines. Returns ErrNotFound / ErrNotDraft accordingly.
func (p *Pool) UpdateDraftDespatch(ctx context.Context, despatchID string, in DespatchUpdateInput) (*model.Despatch, error) {
	var d model.Despatch
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM despatches WHERE id = $1 LIMIT 1`, despatchID,
		).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != "draft" {
			return ErrNotDraft
		}

		set, args := buildDespatchUpdateSet(in)
		set = append(set, "updated_at = now()")
		args = append(args, despatchID)
		sql := fmt.Sprintf(
			"UPDATE despatches SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), despatchColumns,
		)
		if err := scanDespatch(tx.QueryRow(ctx, sql, args...), &d); err != nil {
			return err
		}

		if in.Lines != nil {
			if _, err := tx.Exec(ctx,
				`DELETE FROM despatch_lines WHERE despatch_id = $1`, despatchID,
			); err != nil {
				return err
			}
			if len(in.Lines) > 0 {
				linkRows := make([][]any, 0, len(in.Lines))
				for _, l := range in.Lines {
					linkRows = append(linkRows, []any{
						despatchID, l.LineNumber, l.Description, l.Quantity, l.UnitCode, l.ProductCode,
					})
				}
				if _, err := tx.CopyFrom(ctx,
					pgx.Identifier{"despatch_lines"},
					[]string{"despatch_id", "line_number", "description", "quantity", "unit_code", "product_code"},
					pgx.CopyFromRows(linkRows),
				); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func buildDespatchUpdateSet(in DespatchUpdateInput) ([]string, []any) {
	var set []string
	var args []any
	add := func(col string, val any) {
		args = append(args, val)
		set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if in.IssueDate != nil {
		add("issue_date", *in.IssueDate)
	}
	if in.IssueTime != nil {
		add("issue_time", *in.IssueTime)
	}
	if in.RecipientDocType != nil {
		add("recipient_doc_type", *in.RecipientDocType)
	}
	if in.RecipientDocNumber != nil {
		add("recipient_doc_number", *in.RecipientDocNumber)
	}
	if in.RecipientName != nil {
		add("recipient_name", *in.RecipientName)
	}
	if in.RecipientAddress != nil {
		add("recipient_address", *in.RecipientAddress)
	}
	if in.TransportModality != nil {
		add("transport_modality", *in.TransportModality)
	}
	if in.TransferReason != nil {
		add("transfer_reason", *in.TransferReason)
	}
	if in.TransferReasonDesc != nil {
		add("transfer_reason_desc", *in.TransferReasonDesc)
	}
	if in.StartDate != nil {
		add("start_date", *in.StartDate)
	}
	if in.TotalWeightKg != nil {
		add("total_weight_kg", *in.TotalWeightKg)
	}
	if in.WeightUnitCode != nil {
		add("weight_unit_code", *in.WeightUnitCode)
	}
	if in.TotalPackages != nil {
		add("total_packages", *in.TotalPackages)
	}
	if in.StartUbigeo != nil {
		add("start_ubigeo", *in.StartUbigeo)
	}
	if in.StartAddress != nil {
		add("start_address", *in.StartAddress)
	}
	if in.ArrivalUbigeo != nil {
		add("arrival_ubigeo", *in.ArrivalUbigeo)
	}
	if in.ArrivalAddress != nil {
		add("arrival_address", *in.ArrivalAddress)
	}
	if in.DriverDocType != nil {
		add("driver_doc_type", *in.DriverDocType)
	}
	if in.DriverDocNumber != nil {
		add("driver_doc_number", *in.DriverDocNumber)
	}
	if in.DriverLicense != nil {
		add("driver_license", *in.DriverLicense)
	}
	if in.DriverName != nil {
		add("driver_name", *in.DriverName)
	}
	if in.VehiclePlate != nil {
		add("vehicle_plate", *in.VehiclePlate)
	}
	if in.CarrierRUC != nil {
		add("carrier_ruc", *in.CarrierRUC)
	}
	if in.CarrierName != nil {
		add("carrier_name", *in.CarrierName)
	}
	if in.CarrierMTC != nil {
		add("carrier_mtc", *in.CarrierMTC)
	}
	if in.EventCode != nil {
		add("event_code", *in.EventCode)
	}
	if in.OriginalGreID != nil {
		add("original_gre_id", *in.OriginalGreID)
	}
	if in.RelatedDocType != nil {
		add("related_doc_type", *in.RelatedDocType)
	}
	if in.RelatedDocSeries != nil {
		add("related_doc_series", *in.RelatedDocSeries)
	}
	if in.RelatedDocNumber != nil {
		add("related_doc_number", *in.RelatedDocNumber)
	}
	return set, args
}

// ErrDespatchNotDeletable is returned when DeleteDespatch finds no
// matching draft row — either it doesn't exist or it's past the draft
// stage.
var ErrDespatchNotDeletable = errors.New("despatch not found or not in draft status")

// DespatchIssueResult captures the outcome of sending a GRE to SUNAT.
// All pointer fields are optional — nil leaves the column unchanged.
type DespatchIssueResult struct {
	Status                   string
	SunatTicket              *string
	SunatResponseCode        *string
	SunatResponseDescription *string
	R2XmlKey                 *string
	R2SignedXmlKey           *string
	R2ZipKey                 *string
	R2CdrKey                 *string
	MarkSent                 bool
	MarkAccepted             bool
}

// ApplyDespatchResult writes the pipeline/poll outcome back to
// despatches and returns the refreshed row.
func (p *Pool) ApplyDespatchResult(ctx context.Context, despatchID string, res DespatchIssueResult) (*model.Despatch, error) {
	var d model.Despatch
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		set := []string{"status = $1", "updated_at = now()"}
		args := []any{res.Status}
		add := func(col string, val any) {
			args = append(args, val)
			set = append(set, fmt.Sprintf("%s = $%d", col, len(args)))
		}
		if res.SunatTicket != nil {
			add("sunat_ticket", *res.SunatTicket)
		}
		if res.SunatResponseCode != nil {
			add("sunat_response_code", *res.SunatResponseCode)
		}
		if res.SunatResponseDescription != nil {
			add("sunat_response_description", *res.SunatResponseDescription)
		}
		if res.R2XmlKey != nil {
			add("r2_xml_key", *res.R2XmlKey)
		}
		if res.R2SignedXmlKey != nil {
			add("r2_signed_xml_key", *res.R2SignedXmlKey)
		}
		if res.R2ZipKey != nil {
			add("r2_zip_key", *res.R2ZipKey)
		}
		if res.R2CdrKey != nil {
			add("r2_cdr_key", *res.R2CdrKey)
		}
		if res.MarkSent {
			set = append(set, "sent_at = COALESCE(sent_at, now())")
		}
		if res.MarkAccepted {
			set = append(set, "accepted_at = now()")
		}
		args = append(args, despatchID)
		sql := fmt.Sprintf(
			"UPDATE despatches SET %s WHERE id = $%d RETURNING %s",
			strings.Join(set, ", "), len(args), despatchColumns,
		)
		return scanDespatch(tx.QueryRow(ctx, sql, args...), &d)
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}
