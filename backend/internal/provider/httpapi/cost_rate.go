package httpapi

import (
	"math"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// cost_rate.go saves an administrator-confirmed upstream multiplier for profit
// reporting. The ordinary provider writer atomically patches the selected rate
// and its confirmation evidence into the latest locked configuration, preserving
// credentials, runtime observations, and unrelated concurrent administration.

// CostRateRequest identifies an explicitly verified upstream price multiplier.
type CostRateRequest struct {
	Rate *float64 `json:"rate" binding:"required"` // upstream cost multiplier
}

// SetCostRate validates and confirms a provider's upstream multiplier.
// Explicit zero denotes a free upstream. Only the rate and matching confirmation
// marker are submitted to the existing configuration writer; history is untouched.
func (h *ManagementHandler) SetCostRate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid provider ID")
		return
	}
	var request CostRateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Provide an upstream cost multiplier")
		return
	}
	rate := *request.Rate
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > 999999.9999 || math.Abs(rate*10000-math.Round(rate*10000)) > 0.000001 {
		response.BadRequest(c, "Upstream multiplier must be nonnegative with at most four decimal places")
		return
	}
	row, err := h.adminService.UpdateProvider(c.Request.Context(), id, &provider.UpdateProviderInput{
		RateMultiplier: &rate,
		PatchExtra:     true,
		Extra:          map[string]any{"profit_cost_rate": rate},
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.presenter.Present(c.Request.Context(), row))
}
