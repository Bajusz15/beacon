#!/bin/bash

# Codecov Setup Script for Beacon
# This script helps you set up Codecov for your Beacon repository

echo "🔧 Setting up Codecov for Beacon repository..."

# Check if we're in a git repository
if [ ! -d ".git" ]; then
    echo "❌ Error: Not in a git repository. Please run this script from the root of your Beacon repository."
    exit 1
fi

# Check if codecov.yml already exists
if [ -f "codecov.yml" ]; then
    echo "✅ codecov.yml already exists. Skipping creation."
else
    echo "📝 Creating codecov.yml configuration..."
    cat > codecov.yml << 'EOF'
coverage:
  precision: 2
  round: down
  range: "70...100"

  status:
    project:
      default:
        target: 80%
        threshold: 1%
    patch:
      default:
        target: 80%

comment:
  layout: "reach,diff,files,footer"
  behavior: default
  require_changes: false
  require_base: no
  require_head: yes

github_checks:
  annotations: true
EOF
    echo "✅ Created codecov.yml"
fi

# Check if .github/workflows/ci.yml exists
if [ -f ".github/workflows/ci.yml" ]; then
    echo "✅ CI workflow already exists"
else
    echo "❌ CI workflow not found. Please ensure .github/workflows/ci.yml exists."
    exit 1
fi

echo ""
echo "🎉 Codecov setup complete!"
echo ""
echo "Next steps:"
echo "1. Push your changes to GitHub"
echo "2. Go to https://codecov.io and sign in with GitHub"
echo "3. Add your repository to Codecov"
echo "4. The coverage badge will automatically appear in your README"
echo ""
echo "Your coverage badge URL will be:"
echo "https://codecov.io/gh/YOUR_USERNAME/beacon/branch/main/graph/badge.svg"
echo ""
echo "Replace YOUR_USERNAME with your GitHub username."
