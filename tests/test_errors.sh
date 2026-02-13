#!/bin/bash

# Test error message improvements
echo "Testing error message improvements..."
echo "======================================"
echo ""

# Test 1: Non-existent directory
echo "Test 1: List non-existent directory"
go run main.go <<EOF
list files in /totally/nonexistent/path
exit
EOF

echo ""
echo "Test 2: Read non-existent file"
go run main.go <<EOF
read the file /path/to/nonexistent.txt
exit
EOF

echo ""
echo "Test 3: Patch file that doesn't exist"
go run main.go <<EOF
change "hello" to "goodbye" in nonexistent.txt
exit
EOF

echo ""
echo "Test 4: Run bash command that doesn't exist"
go run main.go <<EOF
run the command nonexistentcommand123
exit
EOF

echo ""
echo "All error tests completed!"
