package xoxno

import (
	"math/big"
)

func minAmountOut(amount *big.Int, slippagePercent float64) *big.Int {
	slipPPM := max(0, min(int64(slippagePercent*10_000), 1_000_000))
	retained := big.NewInt(1_000_000 - slipPPM)
	out := new(big.Int).Mul(amount, retained)
	return out.Quo(out, big.NewInt(1_000_000))
}
