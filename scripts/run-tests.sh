#!/bin/bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}=== Running Scaling Operator Tests ===${NC}\n"

# Function to run a test and capture results
run_test() {
    local test_name=$1
    local test_command=$2
    
    echo -e "${BLUE}Running $test_name...${NC}"
    
    if eval $test_command; then
        echo -e "${GREEN}✓ $test_name passed${NC}\n"
        return 0
    else
        echo -e "${RED}✗ $test_name failed${NC}\n"
        return 1
    fi
}

# Initialize test results
TOTAL_TESTS=0
PASSED_TESTS=0

# Run unit tests
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "Unit Tests" "make test-unit"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
fi

# Run integration tests
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "Integration Tests" "make test-integration"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
fi

# Run envtest
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "Controller Tests (envtest)" "make test-envtest"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
fi

# Run race detection tests
TOTAL_TESTS=$((TOTAL_TESTS + 1))
if run_test "Race Detection Tests" "make test-race"; then
    PASSED_TESTS=$((PASSED_TESTS + 1))
fi

# Generate coverage report
echo -e "${BLUE}Generating coverage report...${NC}"
make test-coverage

# Print summary
echo -e "${YELLOW}=== Test Summary ===${NC}"
echo -e "Total tests: $TOTAL_TESTS"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$((TOTAL_TESTS - PASSED_TESTS))${NC}"

if [ $PASSED_TESTS -eq $TOTAL_TESTS ]; then
    echo -e "\n${GREEN}🎉 All tests passed!${NC}"
    exit 0
else
    echo -e "\n${RED}❌ Some tests failed${NC}"
    exit 1
fi
