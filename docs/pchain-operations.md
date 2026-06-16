# P-Chain Operations Reference

Command names mirror the avalanchego transaction type each one issues. The
"Previous name" column lists names that were **removed in v2.0.0** (no aliases) —
use it to migrate existing scripts.

| Tx Type | Command | SDK Method | Previous name (removed in v2.0.0) |
|---------|---------|------------|-----------------------------------|
| `BaseTx` | `transfer send` | `IssueBaseTx` | — |
| `ExportTx` | `transfer export` | `IssueExportTx` | — |
| `ImportTx` | `transfer import` | `IssueImportTx` | — |
| `AddPermissionlessValidatorTx` | `validator add-permissionless` | `IssueAddPermissionlessValidatorTx` | `validator add` |
| `AddPermissionlessDelegatorTx` | `validator add-permissionless-delegator` | `IssueAddPermissionlessDelegatorTx` | `validator delegate` |
| `CreateSubnetTx` | `subnet create` | `IssueCreateSubnetTx` | — |
| `TransferSubnetOwnershipTx` | `subnet transfer-ownership` | `IssueTransferSubnetOwnershipTx` | — |
| `ConvertSubnetToL1Tx` | `subnet convert-to-l1` | `IssueConvertSubnetToL1Tx` | `subnet convert-l1` |
| `AddSubnetValidatorTx` | `subnet add-validator` | `IssueAddSubnetValidatorTx` | — |
| `RegisterL1ValidatorTx` | `l1 register-validator` | `IssueRegisterL1ValidatorTx` | — |
| `SetL1ValidatorWeightTx` | `l1 set-validator-weight` | `IssueSetL1ValidatorWeightTx` | `l1 set-weight` |
| `IncreaseL1ValidatorBalanceTx` | `l1 increase-validator-balance` | `IssueIncreaseL1ValidatorBalanceTx` | `l1 add-balance` |
| `DisableL1ValidatorTx` | `l1 disable-validator` | `IssueDisableL1ValidatorTx` | — |
| `CreateChainTx` | `chain create` | `IssueCreateChainTx` | — |
