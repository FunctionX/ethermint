// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package app

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
)

type GenesisState map[string]json.RawMessage

// DefaultConsensusParams defines the default Tendermint consensus params used in
// EthermintApp testing.
var DefaultConsensusParams = &tmproto.ConsensusParams{
	Block: &tmproto.BlockParams{
		MaxBytes: 1048576,
		MaxGas:   81500000, // default limit
	},
	Evidence: &tmproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &tmproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

// Setup initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func Setup(isCheckTx bool, patch func(*EthermintApp, GenesisState) GenesisState) *EthermintApp {
	return SetupWithDB(isCheckTx, patch, dbm.NewMemDB())
}

func SetupWithOpts(
	isCheckTx bool,
	patch func(*EthermintApp, GenesisState) GenesisState,
	appOptions simtestutil.AppOptionsMap,
) *EthermintApp {
	return SetupWithDBAndOpts(isCheckTx, patch, dbm.NewMemDB(), appOptions)
}

const ChainID = "ethermint_9000-1"

func SetupWithDB(isCheckTx bool, patch func(*EthermintApp, GenesisState) GenesisState, db dbm.DB) *EthermintApp {
	return SetupWithDBAndOpts(isCheckTx, patch, db, nil)
}

// SetupWithDBAndOpts initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func SetupWithDBAndOpts(
	isCheckTx bool,
	patch func(*EthermintApp, GenesisState) GenesisState,
	db dbm.DB,
	appOptions simtestutil.AppOptionsMap,
) *EthermintApp {
	if appOptions == nil {
		appOptions = make(simtestutil.AppOptionsMap, 0)
	}
	appOptions[server.FlagInvCheckPeriod] = 5
	appOptions[flags.FlagHome] = DefaultNodeHome
	app := NewEthermintApp(log.NewNopLogger(),
		db,
		nil,
		true,
		appOptions,
		baseapp.SetChainID(ChainID),
	)

	if !isCheckTx {
		// init chain must be called to stop deliverState from being nil
		genesisState := NewTestGenesisState(app.AppCodec())
		if patch != nil {
			genesisState = patch(app, genesisState)
		}

		stateBytes, err := json.MarshalIndent(genesisState, "", " ")
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		app.InitChain(
			abci.RequestInitChain{
				ChainId:         ChainID,
				Validators:      []abci.ValidatorUpdate{},
				ConsensusParams: DefaultConsensusParams,
				AppStateBytes:   stateBytes,
			},
		)
	}

	return app
}

// RandomGenesisAccounts is used by the auth module to create random genesis accounts in simulation when a genesis.json is not specified.
// In contrast, the default auth module's RandomGenesisAccounts implementation creates only base accounts and vestings accounts.
func RandomGenesisAccounts(simState *module.SimulationState) authtypes.GenesisAccounts {
	emptyCodeHash := crypto.Keccak256(nil)
	genesisAccs := make(authtypes.GenesisAccounts, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		bacc := authtypes.NewBaseAccountWithAddress(acc.Address)

		ethacc := &ethermint.EthAccount{
			BaseAccount: bacc,
			CodeHash:    common.BytesToHash(emptyCodeHash).String(),
		}
		genesisAccs[i] = ethacc
	}

	return genesisAccs
}

// RandomAccounts creates random accounts with an ethsecp256k1 private key
// TODO: replace secp256k1.GenPrivKeyFromSecret() with similar function in go-ethereum
func RandomAccounts(r *rand.Rand, n int) []simtypes.Account {
	accs := make([]simtypes.Account, n)

	for i := 0; i < n; i++ {
		// don't need that much entropy for simulation
		privkeySeed := make([]byte, 15)
		_, _ = r.Read(privkeySeed)

		prv := secp256k1.GenPrivKeyFromSecret(privkeySeed)
		ethPrv := &ethsecp256k1.PrivKey{}
		_ = ethPrv.UnmarshalAmino(prv.Bytes()) // UnmarshalAmino simply copies the bytes and assigns them to ethPrv.Key
		accs[i].PrivKey = ethPrv
		accs[i].PubKey = accs[i].PrivKey.PubKey()
		accs[i].Address = sdk.AccAddress(accs[i].PubKey.Address())

		accs[i].ConsKey = ed25519.GenPrivKeyFromSecret(privkeySeed)
	}

	return accs
}

// StateFn returns the initial application state using a genesis or the simulation parameters.
// It is a wrapper of simapp.AppStateFn to replace evm param EvmDenom with staking param BondDenom.
func StateFn(cdc codec.JSONCodec, simManager *module.SimulationManager) simtypes.AppStateFn {
	var bondDenom string
	return simtestutil.AppStateFnWithExtendedCbs(
		cdc,
		simManager,
		NewDefaultGenesisState(),
		func(moduleName string, genesisState interface{}) {
			if moduleName == stakingtypes.ModuleName {
				stakingState := genesisState.(*stakingtypes.GenesisState)
				bondDenom = stakingState.Params.BondDenom
			}
		},
		func(rawState map[string]json.RawMessage) {
			evmStateBz, ok := rawState[evmtypes.ModuleName]
			if !ok {
				panic("evm genesis state is missing")
			}

			evmState := new(evmtypes.GenesisState)
			cdc.MustUnmarshalJSON(evmStateBz, evmState)

			// we should replace the EvmDenom with BondDenom
			evmState.Params.EvmDenom = bondDenom

			// change appState back
			rawState[evmtypes.ModuleName] = cdc.MustMarshalJSON(evmState)
		},
	)
}

// NewTestGenesisState generate genesis state with single validator
func NewTestGenesisState(codec codec.Codec) GenesisState {
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	if err != nil {
		panic(err)
	}
	// create validator set with single validator
	validator := tmtypes.NewValidator(pubKey, 1)
	valSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{validator})

	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(100000000000000))),
	}

	genesisState := NewDefaultGenesisState()
	return genesisStateWithValSet(codec, genesisState, valSet, []authtypes.GenesisAccount{acc}, balance)
}

func genesisStateWithValSet(codec codec.Codec, genesisState GenesisState,
	valSet *tmtypes.ValidatorSet, genAccs []authtypes.GenesisAccount,
	balances ...banktypes.Balance,
) GenesisState {
	// set genesis accounts
	authGenesis := authtypes.NewGenesisState(authtypes.DefaultParams(), genAccs)
	genesisState[authtypes.ModuleName] = codec.MustMarshalJSON(authGenesis)

	validators := make([]stakingtypes.Validator, 0, len(valSet.Validators))
	delegations := make([]stakingtypes.Delegation, 0, len(valSet.Validators))

	bondAmt := sdk.DefaultPowerReduction

	for _, val := range valSet.Validators {
		pk, err := cryptocodec.FromTmPubKeyInterface(val.PubKey)
		if err != nil {
			panic(err)
		}
		pkAny, err := codectypes.NewAnyWithValue(pk)
		if err != nil {
			panic(err)
		}
		validator := stakingtypes.Validator{
			OperatorAddress:   sdk.ValAddress(val.Address).String(),
			ConsensusPubkey:   pkAny,
			Jailed:            false,
			Status:            stakingtypes.Bonded,
			Tokens:            bondAmt,
			DelegatorShares:   sdk.OneDec(),
			Description:       stakingtypes.Description{},
			UnbondingHeight:   int64(0),
			UnbondingTime:     time.Unix(0, 0).UTC(),
			Commission:        stakingtypes.NewCommission(sdk.ZeroDec(), sdk.ZeroDec(), sdk.ZeroDec()),
			MinSelfDelegation: sdk.ZeroInt(),
		}
		validators = append(validators, validator)
		delegations = append(delegations, stakingtypes.NewDelegation(genAccs[0].GetAddress(), val.Address.Bytes(), sdk.OneDec()))
	}
	// set validators and delegations
	stakingGenesis := stakingtypes.NewGenesisState(stakingtypes.DefaultParams(), validators, delegations)
	genesisState[stakingtypes.ModuleName] = codec.MustMarshalJSON(stakingGenesis)

	totalSupply := sdk.NewCoins()
	for _, b := range balances {
		// add genesis acc tokens to total supply
		totalSupply = totalSupply.Add(b.Coins...)
	}

	for range delegations {
		// add delegated tokens to total supply
		totalSupply = totalSupply.Add(sdk.NewCoin(sdk.DefaultBondDenom, bondAmt))
	}

	// add bonded amount to bonded pool module account
	balances = append(balances, banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins:   sdk.Coins{sdk.NewCoin(sdk.DefaultBondDenom, bondAmt)},
	})

	// update total supply
	bankGenesis := banktypes.NewGenesisState(
		banktypes.DefaultGenesisState().Params,
		balances,
		totalSupply,
		[]banktypes.Metadata{},
		[]banktypes.SendEnabled{},
	)
	genesisState[banktypes.ModuleName] = codec.MustMarshalJSON(bankGenesis)

	return genesisState
}
