package app

import (
	"context"
	"github.com/DIMO-Network/oracle-example/internal/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/friendsofgo/errors"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// NewAccessMiddleware returns a middleware that check if the wallet in JWT is allowed to access
// Requires JWT middleware to be executed first
func NewAccessMiddleware(access *service.Access) fiber.Handler {
	return func(c *fiber.Ctx) error {
		walletAddress, err := getWalletAddress(c)
		if err != nil {
			return err
		}

		walletsWithAccess, err := access.GetWalletsWithAccess(context.Background())
		if err != nil {
			return err
		}

		if len(walletsWithAccess) == 0 {
			c.Locals("wallet", walletAddress)
			return c.Next()
		}

		for _, wallet := range walletsWithAccess {
			if wallet.Wallet == walletAddress.String() {
				c.Locals("wallet", walletAddress)
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":  "Wallet does not have access.",
			"wallet": walletAddress.String(),
		})
	}
}

func getWalletAddress(c *fiber.Ctx) (common.Address, error) {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	address, ok := claims["ethereum_address"].(string)
	if !ok {
		return common.Address{}, errors.New("wallet_address not found in claims")
	}
	return common.HexToAddress(address), nil
}
