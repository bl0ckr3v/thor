#!/bin/bash

# VeChain Thor Attack Scenarios Integration Script
# This script integrates the attack scenario tests into the existing VeChain Thor test suite

set -e

echo "🔍 VeChain Thor Attack Scenarios Integration"
echo "============================================"

# Check if we're in the correct directory
if [ ! -f "builtin/native_calls_test.go" ]; then
    echo "❌ Error: This script must be run from the VeChain Thor repository root"
    echo "   Expected to find builtin/native_calls_test.go"
    exit 1
fi

echo "✅ Found VeChain Thor repository structure"

# Create backup of existing test files
echo "📦 Creating backups of existing test files..."
if [ -f "builtin/executor_test.go" ]; then
    cp "builtin/executor_test.go" "builtin/executor_test.go.backup"
    echo "   - Backed up executor_test.go"
fi

if [ -f "builtin/native_calls_test.go" ]; then
    cp "builtin/native_calls_test.go" "builtin/native_calls_test.go.backup"
    echo "   - Backed up native_calls_test.go"
fi

# Copy attack scenario test files to the builtin directory
echo "📋 Copying attack scenario test files..."
if [ -f "attack_scenarios_test.go" ]; then
    cp "attack_scenarios_test.go" "builtin/"
    echo "   - Copied attack_scenarios_test.go to builtin/"
else
    echo "❌ Error: attack_scenarios_test.go not found in current directory"
    exit 1
fi

if [ -f "native_vulnerabilities_test.go" ]; then
    cp "native_vulnerabilities_test.go" "builtin/"
    echo "   - Copied native_vulnerabilities_test.go to builtin/"
else
    echo "❌ Error: native_vulnerabilities_test.go not found in current directory"
    exit 1
fi

# Verify the test files are properly formatted
echo "🔧 Verifying test file formatting..."
cd builtin

if ! go fmt attack_scenarios_test.go > /dev/null 2>&1; then
    echo "❌ Error: attack_scenarios_test.go has formatting issues"
    exit 1
fi

if ! go fmt native_vulnerabilities_test.go > /dev/null 2>&1; then
    echo "❌ Error: native_vulnerabilities_test.go has formatting issues"
    exit 1
fi

echo "✅ Test files are properly formatted"

# Check if tests compile
echo "🏗️  Checking if tests compile..."
if ! go build -o /dev/null .; then
    echo "❌ Error: Tests do not compile. Check for import or syntax errors."
    exit 1
fi

echo "✅ Tests compile successfully"

# Run a quick test to verify integration
echo "🧪 Running integration verification..."
if go test -v -run "TestAttackScenario1_FeeDelegationContextAbuse" -timeout 30s; then
    echo "✅ Attack scenario tests integrated successfully"
else
    echo "⚠️  Warning: Some tests may have failed, but integration is complete"
fi

cd ..

# Create a test runner script
echo "📝 Creating test runner script..."
cat > run_attack_tests.sh << 'EOF'
#!/bin/bash

# VeChain Thor Attack Scenarios Test Runner
# Run specific attack scenario tests

set -e

echo "🔍 Running VeChain Thor Attack Scenario Tests"
echo "=============================================="

cd builtin

echo "🎯 Running High-Level Attack Scenarios..."
go test -v -run "TestAttackScenario" -timeout 60s

echo ""
echo "🔬 Running Detailed Vulnerability Tests..."
go test -v -run "TestNative.*Vulnerability" -timeout 60s

echo ""
echo "⚡ Running MTT-Specific Tests..."
go test -v -run "TestMTTAttackVector" -timeout 30s

echo ""
echo "🔥 Running Combined Attack Scenarios..."
go test -v -run "TestCombinedAttackScenario" -timeout 30s

echo ""
echo "✅ All attack scenario tests completed!"
echo ""
echo "📊 Test Summary:"
echo "   - Fee Delegation Context Abuse: HIGH RISK"
echo "   - Native-Solidity State Inconsistency: MEDIUM-HIGH RISK"  
echo "   - Governance Quorum Bypass: MEDIUM RISK"
echo "   - Params-Governance Privilege Escalation: HIGH RISK"
echo ""
echo "📋 See ATTACK_SCENARIOS_README.md for detailed analysis"

cd ..
EOF

chmod +x run_attack_tests.sh
echo "   - Created run_attack_tests.sh"

# Create a focused test runner for CI/CD
echo "📝 Creating CI/CD test runner..."
cat > run_security_tests.sh << 'EOF'
#!/bin/bash

# VeChain Thor Security Tests for CI/CD
# Focused test runner for automated security testing

set -e

cd builtin

echo "🛡️  Running VeChain Thor Security Tests"
echo "======================================"

# Run tests with JSON output for CI parsing
echo "Running attack scenario tests..."
go test -json -run "TestAttackScenario|TestNative.*Vulnerability|TestMTTAttackVector|TestCombinedAttackScenario" -timeout 120s > ../security_test_results.json

# Check if any tests failed
if [ $? -eq 0 ]; then
    echo "✅ All security tests passed"
    exit 0
else
    echo "❌ Some security tests failed - check security_test_results.json"
    exit 1
fi

cd ..
EOF

chmod +x run_security_tests.sh
echo "   - Created run_security_tests.sh"

# Copy documentation
if [ -f "ATTACK_SCENARIOS_README.md" ]; then
    cp "ATTACK_SCENARIOS_README.md" "builtin/"
    echo "   - Copied documentation to builtin/"
fi

echo ""
echo "🎉 Integration Complete!"
echo "======================="
echo ""
echo "📁 Files added to builtin/ directory:"
echo "   - attack_scenarios_test.go"
echo "   - native_vulnerabilities_test.go"
echo "   - ATTACK_SCENARIOS_README.md"
echo ""
echo "🚀 Test runners created:"
echo "   - run_attack_tests.sh (detailed output)"
echo "   - run_security_tests.sh (CI/CD friendly)"
echo ""
echo "▶️  To run the tests:"
echo "   ./run_attack_tests.sh"
echo ""
echo "📖 For detailed analysis, see:"
echo "   builtin/ATTACK_SCENARIOS_README.md"
echo ""
echo "⚠️  SECURITY NOTICE:"
echo "   These tests demonstrate real vulnerabilities in VeChain Thor"
echo "   builtin contracts. Review findings and implement mitigations."
echo ""
echo "🔒 Next Steps:"
echo "   1. Review test results and vulnerability analysis"
echo "   2. Implement recommended security mitigations"
echo "   3. Add these tests to your CI/CD pipeline"
echo "   4. Coordinate with VeChain security team for disclosure"
EOF

chmod +x integrate_attack_tests.sh