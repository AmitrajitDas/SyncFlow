package com.syncflow.auth.config;

import lombok.Data;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.context.annotation.Configuration;

@Data
@Configuration
@ConfigurationProperties(prefix = "jwt")
public class JwtProperties {

    /**
     * Secret key for JWT signing (should be at least 512 bits for HS512)
     */
    private String secret;

    /**
     * Access token expiration time in milliseconds (default: 24 hours)
     */
    private Long expiration;

    /**
     * Refresh token expiration time in milliseconds (default: 7 days)
     */
    private Long refreshExpiration;
}
