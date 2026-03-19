package util

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	TestWoxDataDirEnv  = "WOX_TEST_DATA_DIR"
	TestUserDataDirEnv = "WOX_TEST_USER_DIR"
	TestServerPortEnv  = "WOX_TEST_SERVER_PORT"
)

func GetTestWoxDataDirectoryOverride() string {
	return strings.TrimSpace(os.Getenv(TestWoxDataDirEnv))
}

func GetTestUserDataDirectoryOverride() string {
	return strings.TrimSpace(os.Getenv(TestUserDataDirEnv))
}

func GetTestServerPortOverride() (int, error) {
	portOverride := strings.TrimSpace(os.Getenv(TestServerPortEnv))
	port, err := strconv.Atoi(portOverride)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("invalid %s: %q", TestServerPortEnv, portOverride)
	}

	return port, nil
}
