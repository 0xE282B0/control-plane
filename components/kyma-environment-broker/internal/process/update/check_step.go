package update

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/provisioner"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"
	"github.com/kyma-project/control-plane/components/provisioner/pkg/gqlschema"
)

// CheckStep checks if the SKR is updated
type CheckStep struct {
	provisionerClient   provisioner.Client
	operationManager    *process.OperationManager
	provisioningTimeout time.Duration
}

func NewCheckStep(os storage.Operations,
	provisionerClient provisioner.Client,
	provisioningTimeout time.Duration) *CheckStep {
	return &CheckStep{
		provisionerClient:   provisionerClient,
		operationManager:    process.NewOperationManager(os),
		provisioningTimeout: provisioningTimeout,
	}
}

var _ process.Step = (*CheckStep)(nil)

func (s *CheckStep) Name() string {
	return "Check_Runtime"
}

func (s *CheckStep) Run(operation internal.Operation, log logrus.FieldLogger) (internal.Operation, time.Duration, error) {
	if operation.RuntimeID == "" {
		log.Errorf("Runtime ID is empty")
		return s.operationManager.OperationFailed(operation, "Runtime ID is empty", nil, log)
	}
	return s.checkRuntimeStatus(operation, log.WithField("runtimeID", operation.RuntimeID))
}

func (s *CheckStep) checkRuntimeStatus(operation internal.Operation, log logrus.FieldLogger) (internal.Operation, time.Duration, error) {
	if time.Since(operation.UpdatedAt) > s.provisioningTimeout {
		log.Infof("operation has reached the time limit: updated operation time: %s", operation.UpdatedAt)
		return s.operationManager.OperationFailed(operation, fmt.Sprintf("operation has reached the time limit: %s", s.provisioningTimeout), nil, log)
	}

	if operation.ProvisionerOperationID == "" {
		msg := "Operation does not contain Provisioner Operation ID"
		log.Error(msg)
		return s.operationManager.OperationFailed(operation, msg, nil, log)
	}

	status, err := s.provisionerClient.RuntimeOperationStatus(operation.ProvisioningParameters.ErsContext.GlobalAccountID, operation.ProvisionerOperationID)
	if err != nil {
		log.Errorf("call to provisioner RuntimeOperationStatus failed: %s", err.Error())
		return operation, 1 * time.Minute, nil
	}
	log.Infof("call to provisioner returned %s status", status.State.String())

	var msg string
	if status.Message != nil {
		msg = *status.Message
	}

	switch status.State {
	case gqlschema.OperationStateSucceeded:
		return operation, 0, nil
	case gqlschema.OperationStateInProgress:
		return operation, time.Minute, nil
	case gqlschema.OperationStatePending:
		return operation, time.Minute, nil
	case gqlschema.OperationStateFailed:
		return s.operationManager.OperationFailed(operation, fmt.Sprintf("provisioner client returns failed status: %s", msg), nil, log)
	}

	return s.operationManager.OperationFailed(operation, fmt.Sprintf("unsupported provisioner client status: %s", status.State.String()), nil, log)
}
