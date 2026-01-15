package com.syncflow.auth.service;

import com.syncflow.auth.dto.*;
import com.syncflow.auth.exception.*;
import com.syncflow.auth.model.User;
import com.syncflow.auth.repository.UserRepository;
import com.syncflow.auth.util.JwtTokenProvider;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

@Slf4j
@Service
@RequiredArgsConstructor
public class AuthService {

    private final UserRepository userRepository;
    private final PasswordEncoder passwordEncoder;
    private final JwtTokenProvider jwtTokenProvider;
    private final KafkaTemplate<String, Object> kafkaTemplate;
    private final BucketAssignmentService bucketAssignmentService;

    private static final String USER_CREATED_TOPIC = "user.created";
    private static final String USER_ROLE = "USER";

    /**
     * Register a new user
     *
     * @param request registration request containing username, email, and password
     * @return AuthResponse with tokens and user info
     * @throws UsernameAlreadyExistsException if username is already taken
     * @throws EmailAlreadyExistsException if email is already taken
     */
    @Transactional
    public AuthResponse register(RegisterRequest request) {
        log.info("Registering new user: {}", request.getUsername());

        // Check if username already exists
        if (userRepository.existsByUsername(request.getUsername())) {
            log.warn("Registration failed: username {} already exists", request.getUsername());
            throw new UsernameAlreadyExistsException("Username already exists: " + request.getUsername());
        }

        // Check if email already exists
        if (userRepository.existsByEmail(request.getEmail())) {
            log.warn("Registration failed: email {} already exists", request.getEmail());
            throw new EmailAlreadyExistsException("Email already exists: " + request.getEmail());
        }

        // Create new user
        User user = User.builder()
                .username(request.getUsername())
                .email(request.getEmail())
                .password(passwordEncoder.encode(request.getPassword()))
                .roles(Set.of(USER_ROLE))
                .enabled(true)
                .accountNonExpired(true)
                .accountNonLocked(true)
                .credentialsNonExpired(true)
                .build();

        // Assign bucket to user
        String bucketId = bucketAssignmentService.assignBucket(user.getId(), user.getUsername());
        user.setBucketId(bucketId);

        // Save user to database
        User savedUser = userRepository.save(user);
        log.info("User registered successfully: {} with bucket: {}", savedUser.getUsername(), savedUser.getBucketId());

        // Publish user.created event to Kafka
        publishUserCreatedEvent(savedUser);

        // Generate tokens
        String accessToken = jwtTokenProvider.generateAccessToken(savedUser);
        String refreshToken = jwtTokenProvider.generateRefreshToken(savedUser);

        return AuthResponse.builder()
                .accessToken(accessToken)
                .refreshToken(refreshToken)
                .tokenType("Bearer")
                .expiresIn(jwtTokenProvider.getExpirationInSeconds())
                .user(UserResponse.fromUser(savedUser))
                .build();
    }

    /**
     * Authenticate user and generate tokens
     *
     * @param request login request containing username and password
     * @return AuthResponse with tokens and user info
     * @throws InvalidCredentialsException if credentials are invalid
     */
    @Transactional
    public AuthResponse login(LoginRequest request) {
        log.info("User login attempt: {}", request.getUsername());

        // Find user by username
        User user = userRepository.findByUsername(request.getUsername())
                .orElseThrow(() -> {
                    log.warn("Login failed: user not found: {}", request.getUsername());
                    return new InvalidCredentialsException("Invalid username or password");
                });

        // Validate password
        if (!passwordEncoder.matches(request.getPassword(), user.getPassword())) {
            log.warn("Login failed: invalid password for user: {}", request.getUsername());
            throw new InvalidCredentialsException("Invalid username or password");
        }

        // Check if account is enabled
        if (!user.getEnabled()) {
            log.warn("Login failed: account disabled for user: {}", request.getUsername());
            throw new InvalidCredentialsException("Account is disabled");
        }

        // Update last login timestamp
        user.setLastLoginAt(Instant.now());
        userRepository.save(user);

        log.info("User logged in successfully: {}", user.getUsername());

        // Generate tokens
        String accessToken = jwtTokenProvider.generateAccessToken(user);
        String refreshToken = jwtTokenProvider.generateRefreshToken(user);

        return AuthResponse.builder()
                .accessToken(accessToken)
                .refreshToken(refreshToken)
                .tokenType("Bearer")
                .expiresIn(jwtTokenProvider.getExpirationInSeconds())
                .user(UserResponse.fromUser(user))
                .build();
    }

    /**
     * Refresh access token using refresh token
     *
     * @param refreshTokenRequest request containing refresh token
     * @return AuthResponse with new access token
     * @throws InvalidTokenException if refresh token is invalid
     */
    public AuthResponse refreshToken(RefreshTokenRequest refreshTokenRequest) {
        String refreshToken = refreshTokenRequest.getRefreshToken();
        log.info("Refreshing access token");

        // Validate refresh token
        if (!jwtTokenProvider.validateToken(refreshToken)) {
            log.warn("Token refresh failed: invalid refresh token");
            throw new InvalidTokenException("Invalid or expired refresh token");
        }

        // Check if it's actually a refresh token
        if (!jwtTokenProvider.isRefreshToken(refreshToken)) {
            log.warn("Token refresh failed: not a refresh token");
            throw new InvalidTokenException("Provided token is not a refresh token");
        }

        // Extract username from token
        String username = jwtTokenProvider.getUsernameFromToken(refreshToken);

        // Find user
        User user = userRepository.findByUsername(username)
                .orElseThrow(() -> {
                    log.warn("Token refresh failed: user not found: {}", username);
                    return new UserNotFoundException("User not found: " + username);
                });

        log.info("Access token refreshed successfully for user: {}", username);

        // Generate new access token
        String newAccessToken = jwtTokenProvider.generateAccessToken(user);

        return AuthResponse.builder()
                .accessToken(newAccessToken)
                .refreshToken(refreshToken) // Keep the same refresh token
                .tokenType("Bearer")
                .expiresIn(jwtTokenProvider.getExpirationInSeconds())
                .user(UserResponse.fromUser(user))
                .build();
    }

    /**
     * Get current user details
     *
     * @param username username from JWT token
     * @return UserResponse with user details
     * @throws UserNotFoundException if user not found
     */
    public UserResponse getCurrentUser(String username) {
        log.info("Fetching current user details: {}", username);

        User user = userRepository.findByUsername(username)
                .orElseThrow(() -> {
                    log.warn("User not found: {}", username);
                    return new UserNotFoundException("User not found: " + username);
                });

        return UserResponse.fromUser(user);
    }

    /**
     * Publish user.created event to Kafka
     */
    private void publishUserCreatedEvent(User user) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("userId", user.getId());
            event.put("username", user.getUsername());
            event.put("email", user.getEmail());
            event.put("bucketId", user.getBucketId());
            event.put("roles", user.getRoles());
            event.put("createdAt", user.getCreatedAt());

            kafkaTemplate.send(USER_CREATED_TOPIC, user.getId(), event);
            log.info("Published user.created event to Kafka for user: {}", user.getUsername());
        } catch (Exception e) {
            log.error("Failed to publish user.created event to Kafka for user: {}", user.getUsername(), e);
            // Don't fail the registration if Kafka publish fails
        }
    }
}
