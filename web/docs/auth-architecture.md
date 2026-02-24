# Authentication State Architecture

## Current Approach: AuthContext

The application uses **AuthContext** for authentication state management, not React Query's `useCurrentUserQuery`. This is an intentional architectural decision.

## Access Token Refresh Strategy

Client requests use a two-layer refresh strategy in `web/src/connect.ts`:

1. **Proactive refresh before requests**
- If an access token exists and is expiring soon, the interceptor refreshes it before sending the request.
- This is important for public APIs like `MemoService/ListMemos`: when an expired token is sent, the backend may treat it as anonymous access instead of returning `401`.
- Proactive refresh prevents private/home memo lists from silently degrading to anonymous results after tab focus changes.

2. **Reactive refresh on `401 Unauthenticated`**
- If a protected API returns `401`, the interceptor refreshes and retries once.
- Concurrent refresh calls are deduplicated by a shared refresh manager to avoid refresh-token rotation races.

### Why AuthContext Instead of React Query?

#### 1. **Synchronous Initialization**
- AuthContext fetches user data during app initialization (`main.tsx`)
- Provides synchronous access to `currentUser` throughout the app
- No need to handle loading states in every component

#### 2. **Single Source of Truth**
- User data fetched once on mount
- All components get consistent, up-to-date user info
- No race conditions from multiple query instances

#### 3. **Integration with React Query**
- AuthContext pre-populates React Query cache after fetch (line 81-82 in `AuthContext.tsx`)
- Best of both worlds: synchronous access + cache consistency
- React Query hooks like `useNotifications()` can still use the cached user data

#### 4. **Simpler Component Code**
```typescript
// With AuthContext (current)
const user = useCurrentUser(); // Always returns User | undefined

// With React Query (alternative)
const { data: user, isLoading } = useCurrentUserQuery();
if (isLoading) return <Spinner />;
// Need loading handling everywhere
```

### When to Use React Query for Auth?

Consider migrating auth to React Query if:
- App needs real-time user profile updates from external sources
- Multiple tabs need instant sync
- User data changes frequently during a session

For Memos (a notes app where user profile rarely changes), AuthContext is the right choice.

### Future Considerations

The unused `useCurrentUserQuery()` hook in `useUserQueries.ts` is kept for potential future use. If requirements change (e.g., real-time collaboration on user profiles), migration path is clear:

1. Remove AuthContext
2. Use `useCurrentUserQuery()` everywhere
3. Handle loading states in components
4. Add suspense boundaries if needed

## Recommendation

**Keep the current AuthContext approach.** It provides better DX and performance for this use case.
