package postgres

import (
	"context"
	"database/sql"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/prometheus/client_golang/prometheus"
)

const runtimeMetricsTimeout = 2 * time.Second

// RuntimeMetricsCollector projects retained operational facts without
// exporting source names, resource IDs, queue args, errors or user content.
type RuntimeMetricsCollector struct {
	runtime           *database.Runtime
	sourceRuns        *prometheus.Desc
	sourceDuration    *prometheus.Desc
	sourceRetries     *prometheus.Desc
	jobRuns           *prometheus.Desc
	jobDuration       *prometheus.Desc
	jobRetries        *prometheus.Desc
	jobQueueLag       *prometheus.Desc
	usageCurrent      *prometheus.Desc
	quotaLimit        *prometheus.Desc
	aiCost            *prometheus.Desc
	collectionSuccess *prometheus.Desc
}

func NewRuntimeMetricsCollector(runtime *database.Runtime) *RuntimeMetricsCollector {
	return &RuntimeMetricsCollector{
		runtime:           runtime,
		sourceRuns:        prometheus.NewDesc("hotkey_source_collection_runs_total", "Retained source collection runs by stable source type, outcome and trigger.", []string{"source_type", "outcome", "trigger"}, nil),
		sourceDuration:    prometheus.NewDesc("hotkey_source_collection_duration_seconds", "Retained source collection duration by stable source type, outcome and trigger.", []string{"source_type", "outcome", "trigger"}, nil),
		sourceRetries:     prometheus.NewDesc("hotkey_source_collection_retries_total", "Retained explicitly retried source collection runs by stable source type.", []string{"source_type"}, nil),
		jobRuns:           prometheus.NewDesc("hotkey_job_runs_total", "Retained durable jobs by stable kind and state.", []string{"kind", "state"}, nil),
		jobDuration:       prometheus.NewDesc("hotkey_job_duration_seconds", "Retained final durable job attempt duration by stable kind and state.", []string{"kind", "state"}, nil),
		jobRetries:        prometheus.NewDesc("hotkey_job_retries_total", "Retained durable job retries by stable kind.", []string{"kind"}, nil),
		jobQueueLag:       prometheus.NewDesc("hotkey_job_queue_lag_seconds", "Oldest due durable job queue lag by stable kind.", []string{"kind"}, nil),
		usageCurrent:      prometheus.NewDesc("hotkey_usage_current", "Current safe usage projection by stable dimension and scope.", []string{"dimension", "scope"}, nil),
		quotaLimit:        prometheus.NewDesc("hotkey_quota_limit", "Configured product quota by stable dimension and scope.", []string{"dimension", "scope"}, nil),
		aiCost:            prometheus.NewDesc("hotkey_ai_cost_usd", "Current UTC-day AI cost by stable ledger state.", []string{"state"}, nil),
		collectionSuccess: prometheus.NewDesc("hotkey_runtime_metrics_collection_success", "Whether the latest retained runtime metric projection completed without a database error.", nil, nil),
	}
}

func (collector *RuntimeMetricsCollector) Describe(output chan<- *prometheus.Desc) {
	for _, descriptor := range []*prometheus.Desc{
		collector.sourceRuns, collector.sourceDuration, collector.sourceRetries,
		collector.jobRuns, collector.jobDuration, collector.jobRetries, collector.jobQueueLag,
		collector.usageCurrent, collector.quotaLimit, collector.aiCost, collector.collectionSuccess,
	} {
		output <- descriptor
	}
}

func (collector *RuntimeMetricsCollector) Collect(output chan<- prometheus.Metric) {
	if collector == nil || collector.runtime == nil || collector.runtime.SQL == nil {
		collector.emitCollectionSuccess(output, 0)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeMetricsTimeout)
	defer cancel()
	if err := collector.collectSources(ctx, output); err != nil {
		collector.emitCollectionSuccess(output, 0)
		return
	}
	if err := collector.collectJobs(ctx, output); err != nil {
		collector.emitCollectionSuccess(output, 0)
		return
	}
	if err := collector.collectUsage(ctx, output); err != nil {
		collector.emitCollectionSuccess(output, 0)
		return
	}
	collector.emitCollectionSuccess(output, 1)
}

func (collector *RuntimeMetricsCollector) collectSources(ctx context.Context, output chan<- prometheus.Metric) error {
	rows, err := collector.runtime.SQL.QueryContext(ctx, `
SELECT source.source_type, run.status, run.trigger_type, count(*),
       coalesce(sum(extract(epoch FROM run.finished_at-run.started_at)) FILTER (WHERE run.finished_at IS NOT NULL AND run.started_at IS NOT NULL),0),
       count(*) FILTER (WHERE run.finished_at IS NOT NULL AND run.started_at IS NOT NULL)
FROM collection_runs run
JOIN source_connections source ON source.id=run.source_connection_id
GROUP BY source.source_type, run.status, run.trigger_type
ORDER BY source.source_type, run.status, run.trigger_type`)
	if err != nil {
		return err
	}
	defer rows.Close()
	retries := make(map[string]float64)
	for rows.Next() {
		var sourceType, outcome, trigger string
		var runs, duration float64
		var durationCount uint64
		if err := rows.Scan(&sourceType, &outcome, &trigger, &runs, &duration, &durationCount); err != nil {
			return err
		}
		output <- prometheus.MustNewConstMetric(collector.sourceRuns, prometheus.CounterValue, runs, sourceType, outcome, trigger)
		if durationCount > 0 {
			output <- prometheus.MustNewConstSummary(collector.sourceDuration, durationCount, duration, nil, sourceType, outcome, trigger)
		}
		if trigger == "retry" {
			retries[sourceType] += runs
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for sourceType, count := range retries {
		output <- prometheus.MustNewConstMetric(collector.sourceRetries, prometheus.CounterValue, count, sourceType)
	}
	return nil
}

func (collector *RuntimeMetricsCollector) collectJobs(ctx context.Context, output chan<- prometheus.Metric) error {
	rows, err := collector.runtime.SQL.QueryContext(ctx, `
SELECT kind, state, count(*), coalesce(sum(greatest(attempt-1,0)),0),
       coalesce(sum(extract(epoch FROM finalized_at-attempted_at)) FILTER (WHERE finalized_at IS NOT NULL AND attempted_at IS NOT NULL),0),
       count(*) FILTER (WHERE finalized_at IS NOT NULL AND attempted_at IS NOT NULL),
       coalesce(greatest(max(extract(epoch FROM now()-scheduled_at)) FILTER (WHERE state='available' AND scheduled_at<=now()),0),0)
FROM river_job GROUP BY kind,state ORDER BY kind,state`)
	if err != nil {
		return err
	}
	defer rows.Close()
	retries := make(map[string]float64)
	queueLag := make(map[string]float64)
	for rows.Next() {
		var kind, state string
		var runs, retryCount, duration, lag float64
		var durationCount uint64
		if err := rows.Scan(&kind, &state, &runs, &retryCount, &duration, &durationCount, &lag); err != nil {
			return err
		}
		output <- prometheus.MustNewConstMetric(collector.jobRuns, prometheus.CounterValue, runs, kind, state)
		if durationCount > 0 {
			output <- prometheus.MustNewConstSummary(collector.jobDuration, durationCount, duration, nil, kind, state)
		}
		retries[kind] += retryCount
		if lag > queueLag[kind] {
			queueLag[kind] = lag
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for kind, count := range retries {
		output <- prometheus.MustNewConstMetric(collector.jobRetries, prometheus.CounterValue, count, kind)
		output <- prometheus.MustNewConstMetric(collector.jobQueueLag, prometheus.GaugeValue, queueLag[kind], kind)
	}
	return nil
}

func (collector *RuntimeMetricsCollector) collectUsage(ctx context.Context, output chan<- prometheus.Metric) error {
	var activeMonitors, manualSearches, sourceCalls, aiTokens, deliveries int64
	var reservedCost, settledCost float64
	err := collector.runtime.SQL.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM monitors WHERE deleted_at IS NULL AND status='active'),
  (SELECT coalesce(sum(used),0) FROM quota_usage_ledgers WHERE dimension='manual_searches' AND window_start=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
  (SELECT count(*) FROM collection_runs WHERE created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
  (SELECT coalesce(sum(tokens),0) FROM ai_runs WHERE created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
  (SELECT count(*) FROM alert_email_deliveries WHERE created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')+
  (SELECT count(*) FROM report_deliveries WHERE created_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),
  (SELECT coalesce(sum(reserved_cost),0) FROM ai_budget_ledgers WHERE budget_day=(now() AT TIME ZONE 'UTC')::date),
  (SELECT coalesce(sum(settled_cost),0) FROM ai_budget_ledgers WHERE budget_day=(now() AT TIME ZONE 'UTC')::date)
`).Scan(&activeMonitors, &manualSearches, &sourceCalls, &aiTokens, &deliveries, &reservedCost, &settledCost)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, usage := range []struct {
		dimension string
		scope     string
		value     float64
	}{
		{operationsdomain.DimensionActiveMonitors, "workspace", float64(activeMonitors)},
		{operationsdomain.DimensionManualSearches, "all_users", float64(manualSearches)},
		{operationsdomain.DimensionSourceCalls, "workspace", float64(sourceCalls)},
		{operationsdomain.DimensionAITokens, "workspace", float64(aiTokens)},
		{operationsdomain.DimensionNotificationDelivery, "workspace", float64(deliveries)},
	} {
		output <- prometheus.MustNewConstMetric(collector.usageCurrent, prometheus.GaugeValue, usage.value, usage.dimension, usage.scope)
	}
	output <- prometheus.MustNewConstMetric(collector.quotaLimit, prometheus.GaugeValue, float64(operationsdomain.ActiveMonitorLimit), operationsdomain.DimensionActiveMonitors, "workspace")
	output <- prometheus.MustNewConstMetric(collector.quotaLimit, prometheus.GaugeValue, float64(operationsdomain.ManualSearchDayLimit), operationsdomain.DimensionManualSearches, "user")
	output <- prometheus.MustNewConstMetric(collector.aiCost, prometheus.GaugeValue, reservedCost, "reserved")
	output <- prometheus.MustNewConstMetric(collector.aiCost, prometheus.GaugeValue, settledCost, "settled")
	return nil
}

func (collector *RuntimeMetricsCollector) emitCollectionSuccess(output chan<- prometheus.Metric, value float64) {
	if collector == nil || collector.collectionSuccess == nil {
		return
	}
	output <- prometheus.MustNewConstMetric(collector.collectionSuccess, prometheus.GaugeValue, value)
}
