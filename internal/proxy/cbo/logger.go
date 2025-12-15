// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cbo

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/milvus-io/milvus/pkg/v2/log"
	"github.com/milvus-io/milvus/pkg/v2/util/paramtable"
)

var (
	cboLogger     *zap.Logger
	cboLoggerOnce sync.Once
	cboLogFile    string // Store the log file path for easy access
)

// GetCBOLogFile returns the path to the CBO log file
// Returns empty string if logger hasn't been initialized yet
func GetCBOLogFile() string {
	return cboLogFile
}

// initCBOLogger initializes a separate logger for CBO that writes to a dedicated log file
func initCBOLogger() {
	cboLoggerOnce.Do(func() {
		// Get log root path from config
		rootPath := paramtable.Get().LogCfg.RootPath.GetValue()
		if rootPath == "" {
			// If no root path configured, try to use current working directory
			// or fall back to /tmp/milvus/logs
			if cwd, err := os.Getwd(); err == nil {
				rootPath = filepath.Join(cwd, "logs")
			} else {
				rootPath = "/tmp/milvus/logs"
			}
		}

		// Ensure directory exists
		if err := os.MkdirAll(rootPath, 0o755); err != nil {
			// If we can't create directory, log error and fall back to using default logger
			log.L().Warn("Failed to create CBO log directory, falling back to default logger",
				zap.String("path", rootPath),
				zap.Error(err))
			cboLogger = log.L()
			cboLogFile = "" // No file, using default logger
			return
		}

		// Create CBO-specific log file
		logFilename := filepath.Join(rootPath, "cbo.log")
		cboLogFile = logFilename

		// Use lumberjack for log rotation
		fileWriter := &lumberjack.Logger{
			Filename:   logFilename,
			MaxSize:    300, // MB
			MaxBackups: 20,
			MaxAge:     10, // days
			LocalTime:  true,
		}

		// Create encoder
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
		encoder := zapcore.NewJSONEncoder(encoderConfig)

		// Create core with file writer
		core := zapcore.NewCore(encoder, zapcore.AddSync(fileWriter), zapcore.InfoLevel)

		// Create logger with caller info
		cboLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

		// Log initialization message to both default logger (for visibility) and CBO logger
		log.L().Info("🔥 CBO Logger initialized - CBO logs will be written to dedicated file",
			zap.String("cbo_log_file", logFilename),
			zap.String("log_root_path", rootPath))
		cboLogger.Info("CBO Logger initialized", zap.String("log_file", logFilename))
	})
}

// GetCBOLogger returns the CBO-specific logger
// If not initialized, it will be initialized on first call
func GetCBOLogger() *zap.Logger {
	initCBOLogger()
	return cboLogger
}
