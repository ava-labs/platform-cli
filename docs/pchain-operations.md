# P-Chain Operations Reference

| Operation | Command | SDK Method |
|-----------|---------|------------|
| Send AVAX | `transfer send` | `IssueBaseTx` |
| Export | `transfer export` | `IssueExportTx` |
| Import | `transfer import` | `IssueImportTx` |
| Add Validator | `validator add` | `IssueAddPermissionlessValidatorTx` |
| Add Delegator | `validator delegate` | `IssueAddPermissionlessDelegatorTx` |
| Create Subnet | `subnet create` | `IssueCreateSubnetTx` |
| Transfer Subnet Ownership | `subnet transfer-ownership` | `IssueTransferSubnetOwnershipTx` |
| Convert to L1 | `subnet convert-l1` | `IssueConvertSubnetToL1Tx` |
| Register L1 Validator | `l1 register-validator` | `IssueRegisterL1ValidatorTx` |
| Set L1 Validator Weight | `l1 set-weight` | `IssueSetL1ValidatorWeightTx` |
| Increase L1 Balance | `l1 add-balance` | `IssueIncreaseL1ValidatorBalanceTx` |
| Disable L1 Validator | `l1 disable-validator` | `IssueDisableL1ValidatorTx` |
| Create Chain | `chain create` | `IssueCreateChainTx` |
