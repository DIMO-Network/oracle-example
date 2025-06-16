package onboarding

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/DIMO-Network/go-transactions"
	registry "github.com/DIMO-Network/go-transactions/contracts"
	"github.com/DIMO-Network/oracle-example/internal/config"
	dbmodels "github.com/DIMO-Network/oracle-example/internal/db/models"
	"github.com/DIMO-Network/oracle-example/internal/service"
	"github.com/DIMO-Network/shared/pkg/db"
	"github.com/DIMO-Network/shared/pkg/logfields"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	signer "github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/friendsofgo/errors"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries"
	"math/big"
	"strconv"
	"sync"
	"time"
)

func NewTransactionsClient(settings *config.Settings) (*transactions.Client, error) {
	if settings.RPCURL.String() == "" {
		return nil, errors.New("invalid configuration: missing RPC URL")
	}
	if settings.PaymasterURL.String() == "" {
		return nil, errors.New("invalid configuration: missing Paymaster URL")
	}
	if settings.BundlerURL.String() == "" {
		return nil, errors.New("invalid configuration: missing Bundler URL")
	}
	if len(settings.RegistryAddress.Bytes()) == 0 {
		return nil, errors.New("invalid configuration: missing Registry address")
	}
	if len(settings.VehicleNftAddress.Bytes()) == 0 {
		return nil, errors.New("invalid configuration: missing Vehicle NFT address")
	}
	if len(settings.SyntheticNftAddress.Bytes()) == 0 {
		return nil, errors.New("invalid configuration: missing Synthetic NFT address")
	}
	if settings.ChainID == 0 {
		return nil, errors.New("invalid configuration: missing ChainID")
	}
	if len(settings.DeveloperAAWalletAddress.Bytes()) == 0 {
		return nil, errors.New("invalid configuration: missing Developer AA wallet address")
	}
	pk, err := crypto.HexToECDSA(settings.DeveloperPK)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load developer private key")
	}

	trConfig := transactions.ClientConfig{
		AccountAddress:           settings.DeveloperAAWalletAddress,
		AccountPK:                pk,
		RpcURL:                   &settings.RPCURL,
		PaymasterURL:             &settings.PaymasterURL,
		BundlerURL:               &settings.BundlerURL,
		ChainID:                  new(big.Int).SetInt64(settings.ChainID),
		RegistryAddress:          settings.RegistryAddress,
		VehicleIdAddress:         settings.VehicleNftAddress,
		SyntheticDeviceIdAddress: settings.SyntheticNftAddress,
	}

	transactionsClient, err := transactions.NewClient(&trConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create transactions client")
	}

	return transactionsClient, nil
}

type OnboardingArgs struct {
	Owner     common.Address   `json:"owner"`
	VIN       string           `json:"vin"`
	TypedData signer.TypedData `json:"typedData"`
	Signature hexutil.Bytes    `json:"signature"`
}

func (a OnboardingArgs) Kind() string {
	return "onboard"
}
func (a OnboardingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs: false,
		},
	}
}

type OnboardingWorker struct {
	settings *config.Settings
	logger   zerolog.Logger
	identity service.IdentityAPI
	dbs      *db.Store
	tr       *transactions.Client
	ws       service.SDWalletsAPI
	m        sync.RWMutex
	vendor   VendorOnboardingAPI

	river.WorkerDefaults[OnboardingArgs]
}

func NewOnboardingWorker(settings *config.Settings, logger zerolog.Logger, identity service.IdentityAPI, dbs *db.Store, tr *transactions.Client, ws service.SDWalletsAPI, vendor VendorOnboardingAPI) *OnboardingWorker {
	return &OnboardingWorker{
		settings: settings,
		logger:   logger,
		identity: identity,
		dbs:      dbs,
		tr:       tr,
		ws:       ws,
		vendor:   vendor,
	}
}

func (w *OnboardingWorker) Timeout(*river.Job[OnboardingArgs]) time.Duration { return 30 * time.Minute }

func (w *OnboardingWorker) Work(ctx context.Context, job *river.Job[OnboardingArgs]) error {
	w.logger.Debug().Str(logfields.VIN, job.Args.VIN).Msg("Minting VIN")

	// Check if the VIN record exists
	record, err := w.GetVinRecord(ctx, job.Args.VIN)
	if err != nil {
		return err
	}

	if record.OnboardingStatus < OnboardingStatusVendorValidationSuccess {
		return fmt.Errorf("insufficient verification status")
	}

	// Check onboarding status, if successful, just return
	if record.OnboardingStatus == OnboardingStatusMintSuccess {
		return nil
	}

	// Connect to external vendor
	record, err = w.ConnectToVendorAndUpdate(ctx, record, job.Args)
	if err != nil {
		return err
	}

	// If there's no Vehicle Token ID, mint all
	if !record.VehicleTokenID.Valid {
		w.logger.Debug().Str(logfields.VIN, job.Args.VIN).Msg("No Vehicle Token ID, minting all")
		record, err = w.MintVehicleWithSDAndUpdate(ctx, record, job.Args)
		if err != nil {
			return err
		}
	}

	// If only SD token ID is missing, mint SD
	if !record.SyntheticTokenID.Valid {
		w.logger.Debug().Str(logfields.VIN, job.Args.VIN).Msg("No Synthetic Device Token ID, minting SD")
		_, err = w.MintSDAndUpdate(ctx, record, job.Args)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *OnboardingWorker) GetVinRecord(ctx context.Context, vinToSearch string) (*dbmodels.Vin, error) {
	vin, err := dbmodels.Vins(dbmodels.VinWhere.Vin.EQ(vinToSearch)).One(ctx, w.dbs.DBS().Reader)

	if err != nil {
		return nil, err
	}

	return vin, nil
}

func (w *OnboardingWorker) MintVehicleWithSDAndUpdate(ctx context.Context, record *dbmodels.Vin, args OnboardingArgs) (*dbmodels.Vin, error) {
	// make sure we save status update (and possible new DD)
	defer (func() {
		_ = w.update(ctx, record, args, boil.Whitelist(dbmodels.VinColumns.OnboardingStatus, dbmodels.VinColumns.VehicleTokenID, dbmodels.VinColumns.SyntheticTokenID, dbmodels.VinColumns.WalletIndex))
	})()

	w.logger.Debug().Str(logfields.VIN, args.VIN).Msg("Minting Vehicle with SD")

	deviceDefinition, err := w.identity.FetchDeviceDefinitionByID(record.DeviceDefinitionID.String)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to fetch device definition")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	sdIndex, err := w.GetNextSDWalletIndex(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to get next SD wallet index")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	sdAddress, err := w.ws.GetAddress(sdIndex.NextVal)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to get SD wallet address")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	var integrationOrConnectionID *big.Int
	var sdTypedData *signer.TypedData

	ok := false
	if w.settings.EnableMintingWithConnectionTokenID {
		integrationOrConnectionID, ok = new(big.Int).SetString(w.settings.ConnectionTokenID, 10)
		sdTypedData = w.tr.GetMintVehicleAndSDTypedDataV2(integrationOrConnectionID)
	} else {
		integrationOrConnectionID, ok = new(big.Int).SetString(w.settings.IntegrationTokenID, 10)
		sdTypedData = w.tr.GetMintVehicleAndSDTypedData(integrationOrConnectionID)
	}

	if !ok {
		w.logger.Error().Err(err).Msg("Failed to set integration or connection token ID")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	sdSignature, err := w.ws.SignTypedData(*sdTypedData, sdIndex.NextVal)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to sign SD typed data")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	mintInput := registry.MintVehicleAndSdWithDdInput{
		Owner:               args.Owner,
		VehicleOwnerSig:     args.Signature,
		ManufacturerNode:    new(big.Int).SetUint64(deviceDefinition.Manufacturer.TokenID),
		IntegrationNode:     integrationOrConnectionID,
		DeviceDefinitionId:  deviceDefinition.DeviceDefinitionID,
		SyntheticDeviceAddr: sdAddress,
		SyntheticDeviceSig:  sdSignature,
		AttrInfoPairsDevice: make([]registry.AttributeInfoPair, 0),
		AttrInfoPairsVehicle: []registry.AttributeInfoPair{
			{
				Attribute: "Make",
				Info:      deviceDefinition.Manufacturer.Name,
			},
			{
				Attribute: "Model",
				Info:      deviceDefinition.Model,
			},
			{
				Attribute: "Year",
				Info:      strconv.Itoa(deviceDefinition.Year),
			},
		},
	}
	w.logger.Debug().Str(logfields.VIN, args.VIN).Str(logfields.FunctionName, "MintVehicleWithSDAndUpdate").
		Interface("mintInput", mintInput).Msg("Minting Vehicle with SD Input")

	w.m.Lock()
	_, result, err := w.tr.MintVehicleAndSDWithDD(&mintInput, true, true)
	if err != nil {
		w.m.Unlock()
		w.logger.Error().Err(err).Msg("Failed to mint vehicle and SD")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}
	w.m.Unlock()

	record.WalletIndex = null.Int64From(int64(sdIndex.NextVal))
	record.VehicleTokenID = null.Int64From(result.VehicleId.Int64())
	record.SyntheticTokenID = null.Int64From(result.SyntheticDeviceNode.Int64())
	record.OnboardingStatus = OnboardingStatusMintSuccess

	w.logger.Debug().Str(logfields.VIN, args.VIN).Int64(logfields.VehicleTokenID, record.VehicleTokenID.Int64).Msg("Vehicle minted")
	w.logger.Debug().Str(logfields.VIN, args.VIN).Int64("syntheticDeviceTokenId", record.SyntheticTokenID.Int64).Msg("SD minted")

	return record, nil
}

func (w *OnboardingWorker) MintSDAndUpdate(ctx context.Context, record *dbmodels.Vin, args OnboardingArgs) (vinRecord *dbmodels.Vin, err error) {
	// make sure we save status update (and possible new DD)
	defer (func() {
		_ = w.update(ctx, record, args, boil.Whitelist(dbmodels.VinColumns.OnboardingStatus, dbmodels.VinColumns.SyntheticTokenID, dbmodels.VinColumns.WalletIndex))
	})()

	w.logger.Debug().Str(logfields.VIN, args.VIN).Msg("Minting SD")

	sdIndex, err := w.GetNextSDWalletIndex(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to get next SD wallet index")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	sdAddress, err := w.ws.GetAddress(sdIndex.NextVal)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to get SD wallet address")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	var integrationOrConnectionID *big.Int
	var sdTypedData *signer.TypedData

	ok := false
	if w.settings.EnableMintingWithConnectionTokenID {
		integrationOrConnectionID, ok = new(big.Int).SetString(w.settings.ConnectionTokenID, 10)
		sdTypedData = w.tr.GetMintSDTypedDataV2(integrationOrConnectionID, big.NewInt(record.VehicleTokenID.Int64))
	} else {
		integrationOrConnectionID, ok = new(big.Int).SetString(w.settings.IntegrationTokenID, 10)
		sdTypedData = w.tr.GetMintSDTypedData(integrationOrConnectionID, big.NewInt(record.VehicleTokenID.Int64))
	}

	if !ok {
		w.logger.Error().Err(err).Msg("Failed to set integration or connection token ID")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	sdSignature, err := w.ws.SignTypedData(*sdTypedData, sdIndex.NextVal)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to sign SD typed data")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}

	mintInput := registry.MintSyntheticDeviceInput{
		VehicleOwnerSig:     args.Signature,
		SyntheticDeviceAddr: sdAddress,
		SyntheticDeviceSig:  sdSignature,
		AttrInfoPairs:       make([]registry.AttributeInfoPair, 0),
		IntegrationNode:     integrationOrConnectionID,
		VehicleNode:         big.NewInt(record.VehicleTokenID.Int64),
	}

	w.m.Lock()
	_, result, err := w.tr.MintSD(&mintInput, true, true)
	if err != nil {
		w.m.Unlock()
		w.logger.Error().Err(err).Msg("Failed to mint SD")
		record.OnboardingStatus = OnboardingStatusMintFailure
		return nil, err
	}
	w.m.Unlock()

	record.WalletIndex = null.Int64From(int64(sdIndex.NextVal))
	record.SyntheticTokenID = null.Int64From(result.SyntheticDeviceNode.Int64())
	record.OnboardingStatus = OnboardingStatusMintSuccess

	w.logger.Debug().Str(logfields.VIN, args.VIN).Int64("syntheticDeviceTokenId", record.SyntheticTokenID.Int64).Msg("SD minted")

	return record, nil
}

func (w *OnboardingWorker) ConnectToVendorAndUpdate(ctx context.Context, record *dbmodels.Vin, args OnboardingArgs) (*dbmodels.Vin, error) {
	w.logger.Debug().Str(logfields.VIN, args.VIN).Msg("Connecting to vendor")

	// make sure we save status update (and possible new DD)
	defer (func() { _ = w.update(ctx, record, args, boil.Whitelist(dbmodels.VinColumns.OnboardingStatus)) })()

	record.OnboardingStatus = OnboardingStatusConnectUnknown

	if w.settings.EnableVendorConnection {
		connection, err := w.vendor.Connect([]string{args.VIN})
		if err != nil {
			w.logger.Error().Err(err).Msg("Failed to connect to vendor")
			record.OnboardingStatus = OnboardingStatusConnectFailure
			return nil, err
		}

		w.logger.Debug().Str(logfields.VIN, args.VIN).Interface("connection-result", connection).Msg("Vendor connected")
	} else {
		w.logger.Debug().Str(logfields.VIN, args.VIN).Msg("Vendor connection is disabled, skipping")
	}

	record.OnboardingStatus = OnboardingStatusConnectSuccess

	return record, nil
}

type SDWalletIndex struct {
	NextVal uint32 `boil:"nextval"`
}

func (w *OnboardingWorker) GetNextSDWalletIndex(ctx context.Context) (*SDWalletIndex, error) {
	tx, err := w.dbs.DBS().Writer.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to begin transaction")
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && err == nil {
				w.logger.Error().Err(rbErr).Msg("Failed to rollback transaction")
			}
		}
	}()

	index := SDWalletIndex{}
	qry := fmt.Sprintf("SELECT nextval('%s.sd_wallet_index_seq');", w.settings.DB.Name)
	err = queries.Raw(qry).Bind(ctx, w.dbs.DBS().Reader, &index)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to get next SD wallet index")
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		w.logger.Error().Err(err).Msg("Failed to commit transaction")
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &index, nil
}

func (w *OnboardingWorker) update(ctx context.Context, record *dbmodels.Vin, args OnboardingArgs, columns boil.Columns) error {
	tx, err := w.dbs.DBS().Writer.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to begin transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && err == nil {
				w.logger.Error().Err(rbErr).Msg("Failed to rollback transaction")
			}
		}
	}()

	_, err = record.Update(ctx, w.dbs.DBS().Writer, columns)
	if err != nil {
		w.logger.Error().Err(err).Msg("Failed to update VIN record")
	}

	if err = tx.Commit(); err != nil {
		w.logger.Error().Err(err).Msg("Failed to commit transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	w.logger.Debug().Str(logfields.VIN, args.VIN).Msg("VIN record updated")

	return nil
}
