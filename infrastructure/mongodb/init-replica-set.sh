#!/bin/bash
# MongoDB Replica Set Initialization Script
# This enables Change Streams for CDC

echo "Waiting for MongoDB to start..."
sleep 10

echo "Initializing MongoDB replica set..."
mongosh --host localhost:27017 -u admin -p admin123 --authenticationDatabase admin <<EOF
rs.initiate({
  _id: "rs0",
  members: [
    { _id: 0, host: "mongodb:27017" }
  ]
});
EOF

echo "Replica set initialized successfully!"
echo "Change Streams are now available for CDC."
