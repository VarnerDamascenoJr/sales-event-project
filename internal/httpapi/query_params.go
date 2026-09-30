package httpapi

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/analyticsdb"
)

func parsePagination(c *gin.Context) (int, int, error) {
	page := 1
	pageSize := 20

	if rawPage := c.Query("page"); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil {
			return 0, 0, errValidation("page must be a valid integer")
		}
		page = parsedPage
	}

	if rawPageSize := c.Query("pageSize"); rawPageSize != "" {
		parsedPageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			return 0, 0, errValidation("pageSize must be a valid integer")
		}
		pageSize = parsedPageSize
	}

	if page < 1 {
		return 0, 0, errValidation("page must be greater than or equal to 1")
	}
	if pageSize < 1 {
		return 0, 0, errValidation("pageSize must be greater than or equal to 1")
	}
	if pageSize > 100 {
		return 0, 0, errValidation("pageSize must be less than or equal to 100")
	}

	return page, pageSize, nil
}

func parseAnalyticsExportFilter(c *gin.Context) (analyticsdb.Filter, error) {
	limit := analyticsdb.DefaultLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return analyticsdb.Filter{}, errValidation("limit must be a valid integer")
		}
		limit = parsedLimit
	}

	if limit < 1 {
		return analyticsdb.Filter{}, errValidation("limit must be greater than or equal to 1")
	}
	if limit > analyticsdb.MaxLimit {
		return analyticsdb.Filter{}, errValidation(fmt.Sprintf("limit must be less than or equal to %d", analyticsdb.MaxLimit))
	}

	start := c.Query("start")
	if err := analyticsdb.ValidateTimestampBound(start, "start"); err != nil {
		return analyticsdb.Filter{}, errValidation(err.Error())
	}
	end := c.Query("end")
	if err := analyticsdb.ValidateTimestampBound(end, "end"); err != nil {
		return analyticsdb.Filter{}, errValidation(err.Error())
	}

	return analyticsdb.Filter{
		SalesEventID: c.Query("salesEventId"),
		Start:        start,
		End:          end,
		Limit:        limit,
	}, nil
}
