package metrics

import (
	"context"
	"fmt"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	prometheusNamespace = "compass"
	prometheusSubsystem = "keb"

	resultFailed        float64 = 0
	resultSucceeded     float64 = 1
	resultInProgress    float64 = 2
	resultPending       float64 = 3
	resultCanceling     float64 = 4
	resultCanceled      float64 = 5
	resultRetrying      float64 = 6
	resultUnimplemented float64 = 7
)

type LastOperationState = domain.LastOperationState

const (
	Pending   LastOperationState = "pending"
	Canceling LastOperationState = "canceling"
	Canceled  LastOperationState = "canceled"
	Retrying  LastOperationState = "retrying"
)

// OperationResultCollector provides the following metrics:
// - compass_keb_provisioning_result{"operation_id", "instance_id", "global_account_id", "plan_id"}
// - compass_keb_deprovisioning_result{"operation_id", "instance_id", "global_account_id", "plan_id"}
// - compass_keb_upgrade_result{"operation_id", "instance_id", "global_account_id", "plan_id"}
// These gauges show the status of the operation.
// The value of the gauge could be:
// 0 - Failed
// 1 - Succeeded
// 2 - In progress
// 3 - Pending
// 4 - Canceling
// 5 - Canceled
type OperationResultCollector struct {
	provisioningResultGauge   *prometheus.GaugeVec
	deprovisioningResultGauge *prometheus.GaugeVec
	upgradeKymaResultGauge    *prometheus.GaugeVec
	upgradeClusterResultGauge *prometheus.GaugeVec
}

func NewOperationResultCollector() *OperationResultCollector {
	return &OperationResultCollector{
		provisioningResultGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Subsystem: prometheusSubsystem,
			Name:      "provisioning_result",
			Help:      "Result of the provisioning",
		}, []string{"operation_id", "instance_id", "global_account_id", "plan_id", "error_category", "error_reason"}),
		deprovisioningResultGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Subsystem: prometheusSubsystem,
			Name:      "deprovisioning_result",
			Help:      "Result of the deprovisioning",
		}, []string{"operation_id", "instance_id", "global_account_id", "plan_id", "error_category", "error_reason"}),
		upgradeKymaResultGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Subsystem: prometheusSubsystem,
			Name:      "upgrade_kyma_result",
			Help:      "Result of the kyma upgrade",
		}, []string{"operation_id", "instance_id", "global_account_id", "plan_id"}),
		upgradeClusterResultGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: prometheusNamespace,
			Subsystem: prometheusSubsystem,
			Name:      "upgrade_cluster_result",
			Help:      "Result of the cluster upgrade",
		}, []string{"operation_id", "instance_id", "global_account_id", "plan_id"}),
	}
}

func (c *OperationResultCollector) Describe(ch chan<- *prometheus.Desc) {
	c.provisioningResultGauge.Describe(ch)
	c.deprovisioningResultGauge.Describe(ch)
	c.upgradeKymaResultGauge.Describe(ch)
	c.upgradeClusterResultGauge.Describe(ch)
}

func (c *OperationResultCollector) Collect(ch chan<- prometheus.Metric) {
	c.provisioningResultGauge.Collect(ch)
	c.deprovisioningResultGauge.Collect(ch)
	c.upgradeKymaResultGauge.Collect(ch)
	c.upgradeClusterResultGauge.Collect(ch)
}

func (c *OperationResultCollector) OnUpgradeKymaStepProcessed(ctx context.Context, ev interface{}) error {
	stepProcessed, ok := ev.(process.UpgradeKymaStepProcessed)
	if !ok {
		return fmt.Errorf("expected UpgradeKymaStepProcessed but got %+v", ev)
	}

	resultValue := c.mapResult(stepProcessed.Operation.State)
	op := stepProcessed.Operation
	pp := op.ProvisioningParameters
	c.upgradeKymaResultGauge.
		WithLabelValues(op.Operation.ID, op.InstanceID, pp.ErsContext.GlobalAccountID, pp.PlanID).
		Set(resultValue)

	return nil
}

func (c *OperationResultCollector) OnUpgradeClusterStepProcessed(ctx context.Context, ev interface{}) error {
	stepProcessed, ok := ev.(process.UpgradeClusterStepProcessed)
	if !ok {
		return fmt.Errorf("expected UpgradeClusterStepProcessed but got %+v", ev)
	}

	resultValue := c.mapResult(stepProcessed.Operation.State)
	op := stepProcessed.Operation
	pp := op.ProvisioningParameters
	c.upgradeClusterResultGauge.
		WithLabelValues(op.Operation.ID, op.InstanceID, pp.ErsContext.GlobalAccountID, pp.PlanID).
		Set(resultValue)

	return nil
}

func (c *OperationResultCollector) OnOperationStepProcessed(ctx context.Context, ev interface{}) error {
	e, ok := ev.(process.OperationStepProcessed)
	if !ok {
		return fmt.Errorf("expected OperationStepProcessed but got %+v", ev)
	}

	switch e.Operation.Type {
	case internal.OperationTypeProvision:
		return c.OnProvisioningStepProcessed(ctx, process.ProvisioningStepProcessed{
			StepProcessed: e.StepProcessed,
			Operation:     internal.ProvisioningOperation{Operation: e.Operation},
		})
	case internal.OperationTypeDeprovision:
		return c.OnDeprovisioningStepProcessed(ctx, process.DeprovisioningStepProcessed{
			StepProcessed: e.StepProcessed,
			Operation:     internal.DeprovisioningOperation{Operation: e.Operation},
		})
	default:
		return fmt.Errorf("expected OperationStep of types [%s, %s] but got %+v", internal.OperationTypeProvision, internal.OperationTypeDeprovision, e.Operation.Type)
	}
}

func (c *OperationResultCollector) OnProvisioningSucceeded(ctx context.Context, ev interface{}) error {
	provisioningSucceeded, ok := ev.(process.ProvisioningSucceeded)
	if !ok {
		return fmt.Errorf("expected ProvisioningSucceeded but got %+v", ev)
	}
	op := provisioningSucceeded.Operation
	pp := op.ProvisioningParameters
	c.provisioningResultGauge.WithLabelValues(
		op.ID, op.InstanceID, pp.ErsContext.GlobalAccountID, pp.PlanID, "", "").
		Set(resultSucceeded)

	return nil
}

func (c *OperationResultCollector) OnProvisioningStepProcessed(ctx context.Context, ev interface{}) error {
	stepProcessed, ok := ev.(process.ProvisioningStepProcessed)
	if !ok {
		return fmt.Errorf("expected ProvisioningStepProcessed but got %+v", ev)
	}

	resultValue := c.mapResult(stepProcessed.Operation.State)
	op := stepProcessed.Operation
	pp := op.ProvisioningParameters
	err := op.LastError
	c.provisioningResultGauge.
		WithLabelValues(
			op.ID,
			op.InstanceID,
			pp.ErsContext.GlobalAccountID,
			pp.PlanID,
			string(err.Component()),
			string(err.Reason())).Set(resultValue)

	return nil
}

func (c *OperationResultCollector) OnDeprovisioningStepProcessed(ctx context.Context, ev interface{}) error {
	stepProcessed, ok := ev.(process.DeprovisioningStepProcessed)
	if !ok {
		return fmt.Errorf("expected DeprovisioningStepProcessed but got %+v", ev)
	}

	resultValue := c.mapResult(stepProcessed.Operation.State)
	op := stepProcessed.Operation
	pp := op.ProvisioningParameters
	err := op.LastError
	c.deprovisioningResultGauge.
		WithLabelValues(
			op.ID,
			op.InstanceID,
			pp.ErsContext.GlobalAccountID,
			pp.PlanID,
			string(err.Component()),
			string(err.Reason())).Set(resultValue)
	return nil
}

func (c *OperationResultCollector) mapResult(state domain.LastOperationState) float64 {
	resultValue := resultUnimplemented
	switch state {
	case domain.InProgress:
		resultValue = resultInProgress
	case domain.Succeeded:
		resultValue = resultSucceeded
	case domain.Failed:
		resultValue = resultFailed
	case Pending:
		resultValue = resultPending
	case Canceling:
		resultValue = resultCanceling
	case Canceled:
		resultValue = resultCanceled
	case Retrying:
		resultValue = resultRetrying
	}

	return resultValue
}

func (c *OperationResultCollector) OnOperationSucceeded(ctx context.Context, ev interface{}) error {
	operationSucceeded, ok := ev.(process.OperationSucceeded)
	if !ok {
		return fmt.Errorf("expected OperationSucceeded but got %+v", ev)
	}

	if operationSucceeded.Operation.Type == internal.OperationTypeProvision {
		provisioningOperation := process.ProvisioningSucceeded{
			Operation: internal.ProvisioningOperation{Operation: operationSucceeded.Operation},
		}
		err := c.OnProvisioningSucceeded(ctx, provisioningOperation)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("expected OperationStep of type %s but got %+v", internal.OperationTypeProvision, operationSucceeded.Operation.Type)
	}

	return nil
}
