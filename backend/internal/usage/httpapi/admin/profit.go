package admin

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// profit.go validates administrator profit report ranges before querying usage.
// Explicit dates and timezones must parse successfully; a malformed financial
// filter cannot silently select a different period. Date-only end bounds include
// that local calendar day, matching the shared dashboard date picker.

// GetProfit returns customer margins, separate system costs, and missing-rate reminders.
// Both dates are required and ranges cannot exceed 93 days. The administrator
// route supplies authentication and permission checks before this handler runs.
func (h *DashboardHandler) GetProfit(c *gin.Context) {
	zone := strings.TrimSpace(c.DefaultQuery("timezone", "UTC"))
	location, err := time.LoadLocation(zone)
	if err != nil {
		response.BadRequest(c, "Invalid report timezone")
		return
	}
	start, err := time.ParseInLocation(time.DateOnly, c.Query("start_date"), location)
	if err != nil {
		response.BadRequest(c, "Provide start_date as YYYY-MM-DD")
		return
	}
	end, err := time.ParseInLocation(time.DateOnly, c.Query("end_date"), location)
	if err != nil {
		response.BadRequest(c, "Provide end_date as YYYY-MM-DD")
		return
	}
	// Include the selected local end date using calendar arithmetic, preserving
	// the correct boundary across daylight-saving transitions with unequal day lengths.
	end = end.AddDate(0, 0, 1)
	if !start.Before(end) || end.Sub(start) > 93*24*time.Hour {
		response.BadRequest(c, "Profit date range must be positive and at most 93 days")
		return
	}
	report, err := h.dashboardService.GetProfitReport(c.Request.Context(), start, end, zone)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}
