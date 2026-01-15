package com.syncflow.auth.service;

import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import java.util.UUID;
import java.util.concurrent.atomic.AtomicLong;

@Slf4j
@Service
public class BucketAssignmentService {

    @Value("${bucket.assignment.strategy:round-robin}")
    private String strategy;

    @Value("${bucket.assignment.max-buckets:1000}")
    private int maxBuckets;

    private final AtomicLong counter = new AtomicLong(0);

    /**
     * Assign a bucket ID to a new user based on the configured strategy
     *
     * @param userId the user's ID
     * @param username the user's username
     * @return bucket ID
     */
    public String assignBucket(String userId, String username) {
        String bucketId;

        switch (strategy.toLowerCase()) {
            case "round-robin":
                bucketId = assignRoundRobin();
                break;
            case "random":
                bucketId = assignRandom();
                break;
            case "hash-based":
                bucketId = assignHashBased(userId);
                break;
            default:
                log.warn("Unknown bucket assignment strategy: {}, falling back to round-robin", strategy);
                bucketId = assignRoundRobin();
        }

        log.info("Assigned bucket {} to user {} using strategy {}", bucketId, username, strategy);
        return bucketId;
    }

    /**
     * Round-robin assignment: distribute users evenly across buckets
     */
    private String assignRoundRobin() {
        long count = counter.getAndIncrement();
        long bucketNumber = Math.abs(count % maxBuckets);
        return String.format("bucket-%04d", bucketNumber);
    }

    /**
     * Random assignment: randomly assign users to buckets
     */
    private String assignRandom() {
        int bucketNumber = (int) (Math.random() * maxBuckets);
        return String.format("bucket-%04d", bucketNumber);
    }

    /**
     * Hash-based assignment: use consistent hashing based on user ID
     */
    private String assignHashBased(String userId) {
        int hash = Math.abs(userId.hashCode());
        int bucketNumber = hash % maxBuckets;
        return String.format("bucket-%04d", bucketNumber);
    }

    /**
     * Generate a unique bucket ID (alternative approach)
     */
    public String generateUniqueBucketId() {
        return "bucket-" + UUID.randomUUID().toString();
    }
}
