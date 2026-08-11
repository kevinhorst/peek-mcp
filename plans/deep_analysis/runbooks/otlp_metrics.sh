#!/bin/sh
# SESSION_ID: take the session UUID from events_time.sh output
curl -s -X POST http://127.0.0.1:42422/otlp/v1/metrics \
  -H 'Content-Type: application/json' \
  -d '{
  "resourceMetrics": [
    {
      "scopeMetrics": [
        {
          "metrics": [
            {
              "name": "claude_code.active_time.total",
              "sum": {
                "aggregationTemporality": 1,
                "dataPoints": [
                  {
                    "attributes": [
                      {
                        "key": "session.id",
                        "value": {
                          "stringValue": "'"${SESSION_ID}"'"
                        }
                      }
                    ],
                    "asDouble": 42.5
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}'
