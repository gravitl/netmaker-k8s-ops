#!/bin/bash

# User IP Mapping Example Script
# This script demonstrates how to manage user IP mappings for the Netmaker K8s Proxy

# Configuration
PROXY_URL="http://localhost:8085"  # Adjust this to your proxy URL
PROXY_NAMESPACE="netmaker-k8s-ops-system"  # Adjust this to your namespace

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Netmaker K8s Proxy - User IP Mapping Example${NC}"
echo "=================================================="

# Function to check if proxy is running
check_proxy() {
    echo -e "${YELLOW}Checking if proxy is running...${NC}"
    if curl -s "$PROXY_URL/health" > /dev/null; then
        echo -e "${GREEN}✓ Proxy is running${NC}"
        return 0
    else
        echo -e "${RED}✗ Proxy is not running or not accessible at $PROXY_URL${NC}"
        echo "Please ensure the proxy is running and accessible"
        return 1
    fi
}

# Function to get current mappings
get_mappings() {
    echo -e "${YELLOW}Getting current user IP mappings...${NC}"
    curl -s "$PROXY_URL/admin/user-mappings" | jq '.' 2>/dev/null || echo "Failed to get mappings (jq not installed or invalid JSON)"
}

# Function to add a user mapping
add_mapping() {
    local ip="$1"
    local user="$2"
    local groups="$3"
    
    echo -e "${YELLOW}Adding user IP mapping...${NC}"
    echo "IP: $ip"
    echo "User: $user"
    echo "Groups: $groups"
    
    response=$(curl -s -X POST "$PROXY_URL/admin/user-mappings" \
        -H "Content-Type: application/json" \
        -d "{
            \"ip\": \"$ip\",
            \"user\": \"$user\",
            \"groups\": [$groups]
        }")
    
    echo "Response: $response"
}

# Function to remove a user mapping
remove_mapping() {
    local ip="$1"
    
    echo -e "${YELLOW}Removing user IP mapping for IP: $ip${NC}"
    
    response=$(curl -s -X DELETE "$PROXY_URL/admin/user-mappings/$ip")
    
    echo "Response: $response"
}

# Function to test impersonation
test_impersonation() {
    local ip="$1"
    local expected_user="$2"
    
    echo -e "${YELLOW}Testing impersonation for IP: $ip${NC}"
    echo "Expected user: $expected_user"
    
    # This would require a proper kubeconfig setup
    echo "Note: To test impersonation, you would need to:"
    echo "1. Set up a kubeconfig pointing to the proxy"
    echo "2. Configure the proxy to bind to the WireGuard interface"
    echo "3. Make requests from the mapped IP address"
    echo "4. Check the Kubernetes audit logs for impersonation headers"
}

# Main menu
show_menu() {
    echo ""
    echo "Select an option:"
    echo "1. Check proxy status"
    echo "2. Get current mappings"
    echo "3. Add user mapping"
    echo "4. Remove user mapping"
    echo "5. Test impersonation"
    echo "6. Run example scenarios"
    echo "7. Exit"
    echo ""
    read -p "Enter your choice (1-7): " choice
}

# Example scenarios
run_examples() {
    echo -e "${BLUE}Running example scenarios...${NC}"
    
    # Example 1: Add mappings for different users
    echo -e "\n${YELLOW}Example 1: Adding user mappings${NC}"
    add_mapping "10.0.0.1" "alice" "\"system:authenticated\", \"developers\""
    add_mapping "10.0.0.2" "bob" "\"system:authenticated\", \"admins\""
    add_mapping "10.0.0.3" "charlie" "\"system:authenticated\", \"readonly-users\""
    
    # Example 2: Show current mappings
    echo -e "\n${YELLOW}Example 2: Current mappings${NC}"
    get_mappings
    
    # Example 3: Remove a mapping
    echo -e "\n${YELLOW}Example 3: Removing mapping for 10.0.0.3${NC}"
    remove_mapping "10.0.0.3"
    
    # Example 4: Show updated mappings
    echo -e "\n${YELLOW}Example 4: Updated mappings${NC}"
    get_mappings
}

# Interactive mode
interactive_mode() {
    while true; do
        show_menu
        case $choice in
            1)
                check_proxy
                ;;
            2)
                get_mappings
                ;;
            3)
                read -p "Enter IP address: " ip
                read -p "Enter username: " user
                read -p "Enter groups (comma-separated, quoted): " groups
                add_mapping "$ip" "$user" "$groups"
                ;;
            4)
                read -p "Enter IP address to remove: " ip
                remove_mapping "$ip"
                ;;
            5)
                read -p "Enter IP address to test: " ip
                read -p "Enter expected username: " user
                test_impersonation "$ip" "$user"
                ;;
            6)
                run_examples
                ;;
            7)
                echo -e "${GREEN}Goodbye!${NC}"
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option. Please try again.${NC}"
                ;;
        esac
    done
}

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo -e "${YELLOW}Warning: jq is not installed. JSON output will not be formatted.${NC}"
    echo "Install jq for better output formatting:"
    echo "  Ubuntu/Debian: sudo apt install jq"
    echo "  macOS: brew install jq"
    echo "  CentOS/RHEL: sudo yum install jq"
fi

# Check if curl is installed
if ! command -v curl &> /dev/null; then
    echo -e "${RED}Error: curl is not installed. Please install curl to use this script.${NC}"
    exit 1
fi

# Main execution
if [ "$1" = "--examples" ]; then
    check_proxy && run_examples
elif [ "$1" = "--check" ]; then
    check_proxy
elif [ "$1" = "--get" ]; then
    check_proxy && get_mappings
elif [ "$1" = "--add" ] && [ $# -eq 5 ]; then
    check_proxy && add_mapping "$2" "$3" "$4"
elif [ "$1" = "--remove" ] && [ $# -eq 3 ]; then
    check_proxy && remove_mapping "$2"
else
    interactive_mode
fi
