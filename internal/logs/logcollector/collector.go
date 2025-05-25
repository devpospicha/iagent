/*
SPDX-License-Identifier: GPL-3.0-or-later

Copyright (C) 2025 Aaron Mathis aaron.mathis@gmail.com

This file is part of GoSight.

GoSight is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

GoSight is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with GoSight. If not, see https://www.gnu.org/licenses/.
*/

// gosight/agent/internal/logs/logcollector/collector.go
// Package logcollector provides the Collector interface for all metric collectors.

package logcollector

import (
	"context"

	"github.com/devpospicha/ishared/model"
)

// Collector is an interface that defines the methods for a log collector.
// It includes methods for collecting logs and returning them in a specific format.
type Collector interface {
	Name() string
	Collect(ctx context.Context) ([][]model.LogEntry, error)
}
