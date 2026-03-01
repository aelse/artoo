#!/bin/bash
# Example plugin: datetime
# Returns the current date and time
#
# Usage:
#   - ./datetime --schema        (outputs JSON schema)
#   - ./datetime                 (outputs current date/time)
#
# To install:
#   mkdir -p ~/.artoo/plugins
#   cp datetime ~/.artoo/plugins/
#   chmod +x ~/.artoo/plugins/datetime

if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "datetime",
  "description": "Get the current date and time in various formats",
  "input_schema": {
    "type": "object",
    "properties": {
      "format": {
        "type": "string",
        "description": "Date format: 'iso8601' (default), 'unix', 'rfc2822', or custom strftime format (e.g., '%Y-%m-%d %H:%M:%S')"
      }
    },
    "required": []
  }
}
EOF
    exit 0
fi

# Read input from stdin
INPUT=$(cat)

# Extract format parameter (defaults to iso8601)
FORMAT=$(echo "$INPUT" | jq -r '.format // "iso8601"')

# Return current date/time based on requested format
case "$FORMAT" in
    iso8601)
        date -u "+%Y-%m-%dT%H:%M:%SZ"
        ;;
    unix)
        date +%s
        ;;
    rfc2822)
        date -R
        ;;
    *)
        # Try to use as strftime format
        date +"$FORMAT"
        ;;
esac
