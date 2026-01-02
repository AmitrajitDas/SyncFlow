package com.syncflow.auth.dto;

import com.syncflow.auth.model.User;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.Instant;
import java.util.Set;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class UserResponse {

    private String id;
    private String username;
    private String email;
    private Set<String> roles;
    private String bucketId;
    private Instant createdAt;

    // Factory method to create UserResponse from User entity
    public static UserResponse fromUser(User user) {
        return UserResponse.builder()
                .id(user.getId())
                .username(user.getUsername())
                .email(user.getEmail())
                .roles(user.getRoles())
                .bucketId(user.getBucketId())
                .createdAt(user.getCreatedAt())
                .build();
    }
}
