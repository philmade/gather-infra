/**
 * Authentication module using PocketBase for OAuth
 * PocketNode handles Tinode user sync automatically via hooks
 */

class AuthManager {
    constructor() {
        // PocketBase URL - adjust for production
        this.pocketbaseUrl = this._getPocketBaseUrl();
        this.pb = new PocketBase(this.pocketbaseUrl);

        // Auth state
        this.isAuthenticated = false;
        this.user = null;
        this.tinodeCredentials = null;

        // Callbacks
        this.onAuthSuccess = null;
        this.onAuthError = null;

        console.log('[Auth] Initialized with PocketBase:', this.pocketbaseUrl);
    }

    _getPocketBaseUrl() {
        const hostname = window.location.hostname;
        // Local development
        if (hostname === 'localhost' || hostname === '127.0.0.1') {
            return 'http://localhost:8090';
        }
        // Production: nginx proxies to PocketBase
        return window.location.origin;
    }

    /**
     * Check if user is already authenticated (from stored session)
     * Returns credentials if session is valid
     */
    async checkExistingAuth() {
        if (this.pb.authStore.isValid) {
            console.log('[Auth] Found valid PocketBase session');
            this.user = this.pb.authStore.model;
            this.isAuthenticated = true;

            // Try to get Tinode credentials from cache first
            const stored = localStorage.getItem('tinode_credentials');
            if (stored) {
                this.tinodeCredentials = JSON.parse(stored);
                console.log('[Auth] Restored Tinode credentials from cache');
            } else {
                // Fetch fresh credentials from PocketNode
                try {
                    await this._fetchTinodeCredentials();
                    console.log('[Auth] Fetched fresh Tinode credentials');
                } catch (err) {
                    console.warn('[Auth] Could not fetch Tinode credentials:', err);
                }
            }

            return {
                success: true,
                user: this.user,
                tinodeCredentials: this.tinodeCredentials
            };
        }
        return { success: false };
    }

    /**
     * Sign in with Google OAuth via PocketBase
     * Uses manual redirect flow for better compatibility
     */
    async signInWithGoogle() {
        try {
            console.log('[Auth] Starting Google OAuth flow...');

            // Get the auth methods to get OAuth URL
            const authMethods = await this.pb.collection('users').listAuthMethods();
            const googleProvider = authMethods.oauth2?.providers?.find(p => p.name === 'google');

            if (!googleProvider) {
                throw new Error('Google OAuth not configured in PocketBase');
            }

            // Store the verifier for later
            localStorage.setItem('pb_oauth_verifier', googleProvider.codeVerifier);
            localStorage.setItem('pb_oauth_state', googleProvider.state);
            localStorage.setItem('pb_oauth_provider', 'google');

            // Our app's callback URL (same page)
            const redirectUrl = window.location.origin + window.location.pathname;
            const authUrl = googleProvider.authUrl + encodeURIComponent(redirectUrl);

            console.log('[Auth] Redirect URL:', redirectUrl);
            console.log('[Auth] Full auth URL:', authUrl);

            // Redirect to Google
            window.location.href = authUrl;

            // This won't return - page will redirect
            return { success: false, redirecting: true };

        } catch (error) {
            console.error('[Auth] Google OAuth error:', error);
            throw error;
        }
    }

    /**
     * Handle OAuth callback - call this on page load
     */
    async handleOAuthCallback() {
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        const state = params.get('state');

        console.log('[Auth] Checking for OAuth callback. URL:', window.location.href);
        console.log('[Auth] Code:', code ? 'present' : 'missing', 'State:', state ? 'present' : 'missing');

        if (!code || !state) {
            return { success: false }; // Not an OAuth callback
        }

        // Prevent any redirects while we process
        console.log('[Auth] === OAUTH CALLBACK DETECTED ===');
        console.log('[Auth] Code:', code.substring(0, 30) + '...');
        console.log('[Auth] State:', state);

        const storedState = localStorage.getItem('pb_oauth_state');
        const codeVerifier = localStorage.getItem('pb_oauth_verifier');
        const provider = localStorage.getItem('pb_oauth_provider') || 'google';

        if (!codeVerifier) {
            console.error('[Auth] No code verifier found!');
            throw new Error('OAuth session expired - please try again');
        }

        if (state !== storedState) {
            console.error('[Auth] State mismatch:', state, '!=', storedState);
            throw new Error('OAuth state mismatch - please try again');
        }

        try {
            // The redirect URL must match exactly what we sent
            const redirectUrl = window.location.origin + window.location.pathname;

            console.log('[Auth] Exchanging code for token...');
            console.log('[Auth] Redirect URL:', redirectUrl);

            // Exchange code for token
            const authData = await this.pb.collection('users').authWithOAuth2Code(
                provider,
                code,
                codeVerifier,
                redirectUrl,
                { emailVisibility: true }
            );

            console.log('[Auth] OAuth callback successful:', authData.record.email);

            // Clean up
            localStorage.removeItem('pb_oauth_verifier');
            localStorage.removeItem('pb_oauth_state');
            localStorage.removeItem('pb_oauth_provider');

            // Clear URL params
            window.history.replaceState({}, document.title, window.location.pathname);

            this.user = authData.record;
            this.isAuthenticated = true;

            // Get Tinode credentials from PocketNode
            const tinodeCreds = await this._fetchTinodeCredentials();

            return {
                success: true,
                user: this.user,
                tinodeCredentials: tinodeCreds
            };

        } catch (error) {
            console.error('[Auth] OAuth callback error:', error);
            localStorage.removeItem('pb_oauth_verifier');
            localStorage.removeItem('pb_oauth_state');
            localStorage.removeItem('pb_oauth_provider');
            throw error;
        }
    }

    /**
     * Get Tinode credentials from PocketNode
     * PocketNode automatically creates/syncs Tinode user on auth
     */
    async _fetchTinodeCredentials() {
        try {
            console.log('[Auth] Fetching Tinode credentials...');

            const response = await fetch(`${this.pocketbaseUrl}/api/tinode/credentials`, {
                method: 'GET',
                headers: {
                    'Authorization': this.pb.authStore.token
                }
            });

            if (!response.ok) {
                throw new Error(`Failed to get Tinode credentials: ${response.status}`);
            }

            const data = await response.json();
            console.log('[Auth] Got Tinode credentials');

            // Store credentials
            this.tinodeCredentials = {
                login: data.login,
                password: data.password
            };

            // Cache in localStorage
            localStorage.setItem('tinode_credentials', JSON.stringify(this.tinodeCredentials));

            return this.tinodeCredentials;

        } catch (error) {
            console.error('[Auth] Failed to get Tinode credentials:', error);
            throw error;
        }
    }

    /**
     * Get Tinode credentials for basic auth
     */
    getTinodeCredentials() {
        return this.tinodeCredentials;
    }

    /**
     * Sign up with email and password via PocketBase
     * PocketNode will auto-create Tinode user on first auth
     */
    async signUpWithEmail(email, password, name) {
        try {
            console.log('[Auth] Starting email signup...');

            // Create user in PocketBase
            const userData = {
                email: email,
                password: password,
                passwordConfirm: password,
                name: name
            };

            const record = await this.pb.collection('users').create(userData);
            console.log('[Auth] User created:', record.id);

            // Now authenticate to get a session
            const authData = await this.pb.collection('users').authWithPassword(email, password);
            console.log('[Auth] Authenticated after signup:', authData.record.email);

            this.user = authData.record;
            this.isAuthenticated = true;

            // Get Tinode credentials from PocketNode
            const tinodeCreds = await this._fetchTinodeCredentials();

            return {
                success: true,
                user: this.user,
                tinodeCredentials: tinodeCreds,
                isNewUser: true
            };

        } catch (error) {
            console.error('[Auth] Signup error:', error);

            // Parse PocketBase validation errors
            if (error.data?.data) {
                const errors = error.data.data;
                if (errors.email?.code === 'validation_invalid_email') {
                    throw new Error('Please enter a valid email address');
                }
                if (errors.email?.code === 'validation_not_unique') {
                    throw new Error('This email is already registered. Try signing in instead.');
                }
                if (errors.password?.code) {
                    throw new Error('Password must be at least 8 characters');
                }
            }
            throw error;
        }
    }

    /**
     * Sign in with email and password via PocketBase
     */
    async signInWithEmail(email, password) {
        try {
            console.log('[Auth] Starting email signin...');

            const authData = await this.pb.collection('users').authWithPassword(email, password);
            console.log('[Auth] Email signin successful:', authData.record.email);

            this.user = authData.record;
            this.isAuthenticated = true;

            // Get Tinode credentials from PocketNode
            const tinodeCreds = await this._fetchTinodeCredentials();

            return {
                success: true,
                user: this.user,
                tinodeCredentials: tinodeCreds
            };

        } catch (error) {
            console.error('[Auth] Signin error:', error);
            if (error.status === 400) {
                throw new Error('Invalid email or password');
            }
            throw error;
        }
    }

    /**
     * Sign out from both PocketBase and Tinode
     */
    async signOut() {
        console.log('[Auth] Signing out...');
        this.pb.authStore.clear();
        localStorage.removeItem('tinode_credentials');
        this.isAuthenticated = false;
        this.user = null;
        this.tinodeCredentials = null;
    }

    /**
     * Get current user info
     */
    getCurrentUser() {
        return this.user;
    }

    /**
     * Get display name for current user
     */
    getDisplayName() {
        if (!this.user) return 'Unknown';
        return this.user.name || this.user.username || this.user.email?.split('@')[0] || 'User';
    }

    /**
     * Get avatar URL for current user
     */
    getAvatarUrl() {
        if (!this.user) return null;
        if (this.user.avatar) {
            return this.pb.files.getUrl(this.user, this.user.avatar);
        }
        return null;
    }

    /**
     * Get workspaces the current user is a member of
     * Uses Tinode-native workspaces (grp topics with type=workspace)
     */
    async getWorkspaces() {
        if (!this.user) return [];

        // Use TinodeClient if available (Tinode-native architecture)
        if (window.tinodeClient && window.tinodeClient.isLoggedIn) {
            const workspaces = window.tinodeClient.getWorkspaces();
            console.log('[Auth] Got workspaces from Tinode:', workspaces.length);
            return workspaces;
        }

        console.log('[Auth] TinodeClient not available, no workspaces');
        return [];
    }

    /**
     * Get agents for a specific workspace
     * In Tinode-native architecture, agents are bot users subscribed to the workspace
     * They're discovered via topic subscribers with public.bot = true
     */
    async getWorkspaceAgents(workspaceId) {
        // Use TinodeClient if available
        if (window.tinodeClient && window.tinodeClient.isLoggedIn) {
            try {
                const agents = await window.tinodeClient.getWorkspaceAgents(workspaceId);
                console.log('[Auth] Got agents from Tinode:', agents.length);
                return agents;
            } catch (error) {
                console.warn('[Auth] Failed to get agents from Tinode:', error);
                return [];
            }
        }

        console.log('[Auth] TinodeClient not available, no agents');
        return [];
    }

    /**
     * Create a new workspace
     * Uses Tinode-native workspaces (creates grp topic with type=workspace)
     */
    async createWorkspace(name, slug, description = '') {
        if (!this.user) throw new Error('Not authenticated');

        // Use TinodeClient if available (Tinode-native architecture)
        if (!window.tinodeClient || !window.tinodeClient.isLoggedIn) {
            throw new Error('Not connected to Tinode. Please refresh the page.');
        }

        try {
            const normalizedSlug = slug || name.toLowerCase().replace(/\s+/g, '-');
            const workspace = await window.tinodeClient.createWorkspace(name, normalizedSlug, description);
            console.log('[Auth] Created workspace via Tinode:', workspace);
            return workspace;
        } catch (error) {
            console.error('[Auth] Failed to create workspace:', error);
            throw error;
        }
    }

    /**
     * Create workspace gateway
     * TODO: In new architecture, workspaces are Tinode grp topics - no separate gateway needed
     */
    async _createWorkspaceGateway(workspaceId, workspaceName) {
        // In Tinode-native architecture, workspaces ARE Tinode topics
        // No separate gateway needed - agents subscribe directly to workspace topic
        console.log('[Auth] Workspace gateway creation skipped (Tinode-native mode)');
        return { success: true, gateway_id: workspaceId };
    }

    /**
     * Get available tools from the tools library
     */
    async getToolsLibrary() {
        try {
            const tools = await this.pb.collection('tools_library').getFullList({
                filter: 'enabled = true'
            });

            return tools.map(t => ({
                name: t.name,
                displayName: t.display_name,
                description: t.description,
                category: t.category,
                schema: t.function_schema
            }));
        } catch (error) {
            // Collection might not exist yet
            if (error.status === 404) {
                console.log('[Auth] tools_library collection not created yet');
                return [];
            }
            console.error('[Auth] Failed to fetch tools library:', error);
            return [];
        }
    }

    /**
     * Generate an invite link for a workspace
     * Creates an invite code in PocketBase that can be used to join
     */
    async generateInviteLink(workspaceId, workspaceName) {
        if (!this.user) throw new Error('Not authenticated');

        try {
            // Generate a random invite code
            const inviteCode = this._generateInviteCode();

            // Find channel IDs to include in invite
            let channelIds = [];
            if (window.tinodeClient?.findWorkspaceChannels) {
                const channels = await window.tinodeClient.findWorkspaceChannels(workspaceId);
                channelIds = channels.map(c => c.id);
                console.log('[Auth] Including channels in invite:', channelIds);
            }

            // Create invite record in PocketBase
            const inviteData = {
                code: inviteCode,
                workspace_id: workspaceId,
                workspace_name: workspaceName,
                created_by: this.user.id,
                expires_at: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(), // 7 days
                uses: 0,
                max_uses: 0 // 0 = unlimited
            };

            // Add channel_ids if the field exists in the schema
            if (channelIds.length > 0) {
                inviteData.channel_ids = JSON.stringify(channelIds);
            }

            const invite = await this.pb.collection('workspace_invites').create(inviteData);

            const inviteUrl = `${window.location.origin}/join/${inviteCode}`;
            console.log('[Auth] Generated invite link:', inviteUrl);

            return {
                code: inviteCode,
                url: inviteUrl,
                expiresAt: invite.expires_at,
                channelIds: channelIds
            };
        } catch (error) {
            console.error('[Auth] Failed to generate invite:', error);
            // If collection doesn't exist, return a placeholder
            if (error.status === 404) {
                console.warn('[Auth] workspace_invites collection not found - invite links disabled');
                return null;
            }
            throw error;
        }
    }

    /**
     * Get existing invite link for a workspace (or create one)
     */
    async getOrCreateInviteLink(workspaceId, workspaceName) {
        if (!this.user) throw new Error('Not authenticated');

        try {
            // Try to find existing valid invite
            const existing = await this.pb.collection('workspace_invites').getFirstListItem(
                `workspace_id = "${workspaceId}" && expires_at > @now`,
                { sort: '-created' }
            );

            if (existing) {
                return {
                    code: existing.code,
                    url: `${window.location.origin}/join/${existing.code}`,
                    expiresAt: existing.expires_at
                };
            }
        } catch (e) {
            // No existing invite found, create new one
        }

        return this.generateInviteLink(workspaceId, workspaceName);
    }

    /**
     * Join a workspace using an invite code
     * This is now a thin wrapper around handleInviteFlow for backwards compatibility
     */
    async joinWorkspaceByInvite(inviteCode) {
        return this.handleInviteFlow(inviteCode);
    }

    /**
     * CONSOLIDATED INVITE HANDLER
     *
     * Handles the complete invite flow in one idempotent function:
     * 1. Validates the invite code
     * 2. Joins the workspace (or confirms already joined)
     * 3. Finds and joins the main channel
     * 4. Returns workspace members with proper names
     *
     * This function is idempotent - calling it multiple times is safe.
     * If already joined, it will just ensure channels are joined.
     */
    async handleInviteFlow(inviteCode) {
        if (!this.user) throw new Error('Not authenticated');

        const tinode = window.tinodeClient;
        if (!tinode || !tinode.isLoggedIn) {
            throw new Error('Not connected to chat. Please refresh and try again.');
        }

        console.log('[InviteFlow] Starting invite flow for code:', inviteCode);

        // Step 1: Validate the invite
        let invite;
        try {
            invite = await this.pb.collection('workspace_invites').getFirstListItem(
                `code = "${inviteCode}" && expires_at > @now`
            );
        } catch (e) {
            throw new Error('Invalid or expired invite code');
        }

        if (!invite) {
            throw new Error('Invalid or expired invite code');
        }

        console.log('[InviteFlow] Valid invite for workspace:', invite.workspace_name);

        // Step 2: Join the workspace (idempotent - safe to call if already joined)
        const workspaceTopic = tinode.client.getTopic(invite.workspace_id);
        try {
            await workspaceTopic.subscribe(
                workspaceTopic.startMetaQuery().withDesc().withSub().build()
            );
            console.log('[InviteFlow] Subscribed to workspace');
        } catch (e) {
            // May already be subscribed, that's fine
            console.log('[InviteFlow] Workspace subscribe:', e.message);
        }

        // Step 3: Get workspace members (with proper names)
        const members = [];
        await new Promise(r => setTimeout(r, 500)); // Let subscriber data load

        workspaceTopic.subscribers?.((sub) => {
            if (sub.user && sub.user !== tinode.myUserId) {
                members.push({
                    id: sub.user,
                    name: sub.public?.fn || sub.user,
                    online: sub.online || false,
                    isBot: sub.public?.bot || false
                });
            }
        });
        console.log('[InviteFlow] Found workspace members:', members.length);

        // Step 4: Find channels for this workspace
        let mainChannelId = null;
        let channels = [];

        console.log('[InviteFlow] Looking for channels in workspace:', invite.workspace_id);

        // First, check if invite has channel_ids stored (most reliable)
        if (invite.channel_ids) {
            try {
                const channelIds = JSON.parse(invite.channel_ids);
                console.log('[InviteFlow] Invite has stored channel IDs:', channelIds);
                for (const channelId of channelIds) {
                    channels.push({
                        id: channelId,
                        name: 'channel', // Name will be updated after joining
                        subscribed: false
                    });
                }
            } catch (e) {
                console.log('[InviteFlow] Could not parse channel_ids:', e.message);
            }
        }

        // If no channels from invite, try to find them
        if (channels.length === 0 && tinode.findWorkspaceChannels) {
            channels = await tinode.findWorkspaceChannels(invite.workspace_id);
        }

        // Last resort: check meTopic contacts
        if (channels.length === 0 && tinode.meTopic) {
            tinode.meTopic.contacts?.((contact) => {
                if (!contact.topic?.startsWith('grp')) return;
                const pub = contact.public;
                if (pub?.type === 'channel' && pub?.parent === invite.workspace_id) {
                    channels.push({
                        id: contact.topic,
                        name: pub?.name || pub?.fn || 'channel',
                        subscribed: true
                    });
                }
            });
        }

        console.log('[InviteFlow] Found', channels.length, 'channels');

        // Step 5: Join ALL channels (not just main)
        for (const channel of channels) {
            console.log('[InviteFlow] Attempting to join channel:', channel.id);

            try {
                const channelTopic = tinode.client.getTopic(channel.id);
                await channelTopic.subscribe(
                    channelTopic.startMetaQuery().withDesc().withSub().withLaterData(50).build()
                );

                // Get the channel name from metadata
                const name = channelTopic.public?.name || channelTopic.public?.fn || channel.name;
                console.log('[InviteFlow] Joined channel:', name);

                // Set main channel to the first one or one named "main"
                if (!mainChannelId || name.toLowerCase() === 'main') {
                    mainChannelId = channel.id;
                }
            } catch (e) {
                console.log('[InviteFlow] Could not join channel', channel.id, ':', e.message);
            }
        }

        if (!mainChannelId && channels.length > 0) {
            mainChannelId = channels[0].id;
        }

        console.log('[InviteFlow] Main channel:', mainChannelId);

        // Step 7: Increment invite uses (non-critical)
        try {
            await this.pb.collection('workspace_invites').update(invite.id, {
                uses: (invite.uses || 0) + 1
            });
        } catch (e) {
            // Ignore - may not have permission
        }

        // Step 8: Refresh meTopic to update subscriptions list
        if (tinode.meTopic) {
            try {
                await tinode.meTopic.getMeta(
                    tinode.meTopic.startMetaQuery().withSub().build()
                );
            } catch (e) {
                // Non-critical
            }
        }

        console.log('[InviteFlow] Complete!');

        return {
            success: true,
            workspaceId: invite.workspace_id,
            workspaceName: invite.workspace_name,
            mainChannelId: mainChannelId,
            members: members,
            channels: channels
        };
    }

    /**
     * Generate a random invite code
     */
    _generateInviteCode() {
        const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // Removed confusing chars like 0,O,1,I
        let code = '';
        for (let i = 0; i < 8; i++) {
            code += chars.charAt(Math.floor(Math.random() * chars.length));
        }
        return code;
    }

    /**
     * Look up invite info by code (public, no auth required)
     * Returns workspace name and validity
     */
    async lookupInvite(inviteCode) {
        try {
            const invite = await this.pb.collection('workspace_invites').getFirstListItem(
                `code = "${inviteCode}" && expires_at > @now`
            );

            return {
                valid: true,
                workspaceName: invite.workspace_name,
                workspaceId: invite.workspace_id,
                code: invite.code
            };
        } catch (error) {
            console.log('[Auth] Invite lookup failed:', error.message);
            return {
                valid: false,
                workspaceName: null,
                workspaceId: null,
                code: inviteCode
            };
        }
    }

    /**
     * Search for users to invite
     */
    async searchUsers(query) {
        if (!query || query.length < 2) return [];

        try {
            const users = await this.pb.collection('users').getList(1, 10, {
                filter: `name ~ "${query}" || email ~ "${query}"`,
                fields: 'id,name,email,avatar'
            });

            return users.items.map(u => ({
                id: u.id,
                name: u.name || u.email.split('@')[0],
                email: u.email,
                avatar: u.avatar ? this.pb.files.getUrl(u, u.avatar) : null
            }));
        } catch (error) {
            console.error('[Auth] User search failed:', error);
            return [];
        }
    }

    /**
     * Invite users to a workspace by adding them directly
     */
    async inviteUsersToWorkspace(workspaceId, userIds) {
        if (!this.user) throw new Error('Not authenticated');

        // Use Tinode to add users to workspace
        if (!window.tinodeClient || !window.tinodeClient.isLoggedIn) {
            throw new Error('Not connected to chat. Please refresh and try again.');
        }

        const results = [];
        for (const userId of userIds) {
            try {
                await window.tinodeClient.addUserToWorkspace(workspaceId, userId);
                results.push({ userId, success: true });
            } catch (error) {
                results.push({ userId, success: false, error: error.message });
            }
        }

        return results;
    }

    /**
     * Create a new agent in a workspace
     */
    async createAgent(workspaceId, agentData) {
        if (!this.user) throw new Error('Not authenticated');

        try {
            const agent = await this.pb.collection('workspace_agents').create({
                workspace: workspaceId,
                handle: agentData.handle,
                display_name: agentData.displayName,
                description: agentData.description || '',
                agent_type: agentData.agentType || 'managed',
                prompt: agentData.prompt || '',
                model: agentData.model || 'gemini-2.5-flash',
                tools: agentData.tools || [],
                endpoint_url: agentData.endpointUrl || '',
                timeout_ms: agentData.timeoutMs || 30000,
                deployed_by: this.user.id,
                status: 'running'
            });

            return {
                id: agent.id,
                handle: agent.handle,
                displayName: agent.display_name,
                description: agent.description,
                agentType: agent.agent_type,
                model: agent.model,
                tools: agent.tools || [],
                status: agent.status,
                deployedBy: this.user.name || this.user.email,
                deployedAt: agent.created
            };
        } catch (error) {
            console.error('[Auth] Failed to create agent:', error);
            if (error.status === 400) {
                // Handle validation errors
                const message = error.data?.data?.handle?.message || error.message;
                throw new Error(message);
            }
            throw error;
        }
    }
}

// Export singleton instance
window.authManager = new AuthManager();
