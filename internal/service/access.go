package service

import (
	"context"
	"database/sql"
	dbmodels "github.com/DIMO-Network/oracle-example/internal/db/models"
	"github.com/DIMO-Network/shared/pkg/db"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"time"
)

type Access struct {
	pdb    *db.Store
	logger *zerolog.Logger
	cache  *cache.Cache
}

func NewAccessService(pdb *db.Store, logger *zerolog.Logger) *Access {
	c := cache.New(10*time.Minute, 15*time.Minute)

	return &Access{
		pdb:    pdb,
		logger: logger,
		cache:  c,
	}
}

func (a *Access) GetWalletsWithAccess(ctx context.Context) (dbmodels.AccessSlice, error) {
	if cachedResponse, found := a.cache.Get("access"); found {
		return cachedResponse.(dbmodels.AccessSlice), nil
	}

	tx, err := a.pdb.DBS().Writer.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		a.logger.Error().Err(err).Msg("GetWalletsWithAccess: Failed to begin transaction for access wallets")
		return nil, err
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				a.logger.Error().Err(rbErr).Msg("GetWalletsWithAccess: Failed to rollback transaction for access wallets")
			}
		} else {
			if cmErr := tx.Commit(); cmErr != nil {
				a.logger.Error().Err(cmErr).Msg("GetWalletsWithAccess: Failed to commit transaction for access wallets")
			}
		}
	}()

	wallets, err := dbmodels.Accesses().All(ctx, tx)
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to fetch access wallets")
		return nil, err
	}

	a.cache.Set("access", wallets, cache.DefaultExpiration)

	return wallets, nil
}
