package geth

import (
	"bytes"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"

	evm "github.com/evmos/ethermint/x/evm/vm"
)

var (
	_ evm.EVM         = (*EVM)(nil)
	_ evm.Constructor = NewEVM
)

// EVM is the wrapper for the go-ethereum EVM.
type EVM struct {
	*vm.EVM
	precompiles       evm.PrecompiledContracts
	activePrecompiles []common.Address
}

// NewEVM defines the constructor function for the go-ethereum (geth) EVM. It uses
// the default precompiled contracts and the EVM concrete implementation from
// geth.
func NewEVM(
	ctx sdk.Context,
	blockCtx vm.BlockContext,
	txCtx vm.TxContext,
	stateDB vm.StateDB,
	chainConfig *params.ChainConfig,
	config vm.Config,
	getPrecompilesExtended func(ctx sdk.Context, evm *vm.EVM) evm.PrecompiledContracts,
) evm.EVM {
	newEvm := &EVM{
		EVM: vm.NewEVM(blockCtx, txCtx, stateDB, chainConfig, config),
	}

	rules := chainConfig.Rules(blockCtx.BlockNumber, blockCtx.Random != nil)
	precompiles := vm.DefaultPrecompiles(rules)
	activePrecompiles := vm.DefaultActivePrecompiles(rules)

	customPrecompiles := getPrecompilesExtended(ctx, newEvm.EVM)
	for k, v := range customPrecompiles {
		precompiles[k] = v
		activePrecompiles = append(activePrecompiles, v.Address())
	}

	sort.SliceStable(activePrecompiles, func(i, j int) bool {
		return bytes.Compare(activePrecompiles[i].Bytes(), activePrecompiles[j].Bytes()) < 0
	})

	newEvm.precompiles = precompiles
	newEvm.activePrecompiles = activePrecompiles

	return newEvm
}

// Context returns the EVM's Block Context
func (e EVM) Context() vm.BlockContext {
	return e.EVM.Context
}

// TxContext returns the EVM's Tx Context
func (e EVM) TxContext() vm.TxContext {
	return e.EVM.TxContext
}

// Config returns the configuration options for the EVM.
func (e EVM) Config() vm.Config {
	return e.EVM.Config
}

// Precompile returns the precompiled contract associated with the given address
// and the current chain configuration. If the contract cannot be found it returns
// nil.
func (e EVM) Precompile(addr common.Address) (p vm.PrecompiledContract, found bool) {
	p, found = e.precompiles[addr]
	return p, found
}

// ActivePrecompiles returns a list of all the active precompiled contract addresses
// for the current chain configuration.
func (e *EVM) ActivePrecompiles(_ params.Rules) []common.Address {
	return e.activePrecompiles
}
