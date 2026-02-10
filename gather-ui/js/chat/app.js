/**
 * Main Application
 * Coordinates all components and manages application state
 */

class App {
    constructor() {
        // Auto-detect production vs development
        const hostname = window.location.hostname;
        const isProduction = hostname !== 'localhost' && hostname !== '127.0.0.1';

        this.config = {
            // In production, nginx proxies WebSocket at same origin
            // In dev, connect directly to Tinode
            host: isProduction ? window.location.host : 'localhost:6060',
            secure: isProduction || window.location.protocol === 'https:',
            apiKey: 'AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K'
        };

        // Components
        this.tinode = null;
        this.sidebar = null;
        this.messages = null;
        this.composer = null;
        this.presence = null;
        this.search = null;

        // State
        this.currentUser = null;
        this.currentTopic = null;
        this.currentWorkspace = null;
        this.selectedAgent = null;

        // Mobile state
        this.isMobile = window.innerWidth < 1024;
        this.sidebarOpen = false;

        // DOM elements
        this.loginModal = document.getElementById('login-modal');
        this.loginForm = document.getElementById('login-form');
        this.loginError = document.getElementById('login-error');
        this.signupForm = document.getElementById('signup-form');
        this.signupError = document.getElementById('signup-error');
        this.appContainer = document.getElementById('app-container');
        this.createGroupModal = document.getElementById('create-group-modal');
        this.createGroupForm = document.getElementById('create-group-form');
        this.rightPanel = document.getElementById('right-panel');
        this.closePanelBtn = document.getElementById('close-panel-btn');
        this.panelTitle = document.getElementById('panel-title');
        this.panelContent = document.getElementById('panel-content');
        this.sdkSettingsModal = document.getElementById('sdk-settings-modal');
        this.createAgentModal = document.getElementById('create-agent-modal');
        this.createAgentForm = document.getElementById('create-agent-form');
        this.onboardingModal = document.getElementById('onboarding-modal');
        this.onboardingForm = document.getElementById('onboarding-form');
        this.joinWorkspaceModal = document.getElementById('join-workspace-modal');
        this.inviteModal = document.getElementById('invite-modal');

        // Auth state
        this.authMode = 'signin'; // 'signin' or 'signup'
        this.isNewUser = false;

        // Tools library cache
        this.toolsLibrary = [];
    }

    async init() {
        // Initialize components
        this.sidebar = new Sidebar();
        this.messages = new Messages();
        this.composer = new Composer();
        this.presence = new Presence();
        this.search = new Search();

        // Initialize Tinode client
        this.tinode = new TinodeClient(this.config);
        this.tinode.init();

        // Expose globally for auth.js workspace methods
        window.tinodeClient = this.tinode;

        // Setup handlers
        this._setupTinodeEvents();
        this._setupUIEvents();
        this._setupComponentCallbacks();

        // Connect to Tinode
        try {
            await this.tinode.connect();
        } catch (err) {
            this._showLoginError('Failed to connect to server');
        }

        // Check for OAuth callback first
        await this._checkOAuthCallback();

        // If not an OAuth callback, try to restore existing session
        if (!this.currentUser) {
            await this._tryRestoreSession();
        }

        // Check for invite link in URL (e.g., /join/ABC123)
        await this._checkInviteUrl();
    }

    /**
     * Check if URL contains an invite code
     */
    async _checkInviteUrl() {
        const path = window.location.pathname;
        const match = path.match(/^\/join\/([A-Za-z0-9]+)$/);

        if (match) {
            const inviteCode = match[1].toUpperCase();
            console.log('[App] Detected invite code in URL:', inviteCode);

            // Store the invite code to use after login
            storage.set('pendingInviteCode', inviteCode);

            // Look up invite info to show workspace name
            const inviteInfo = await window.authManager.lookupInvite(inviteCode);

            if (inviteInfo.valid) {
                // Store workspace name for display
                storage.set('pendingInviteWorkspace', inviteInfo.workspaceName);

                // Update login modal to show invite context
                this._showInviteLoginMode(inviteInfo.workspaceName);
            }

            // Clear the URL but keep user on page
            window.history.replaceState({}, document.title, '/');

            // If already logged in, join immediately
            if (this.currentUser) {
                this._handlePendingInvite();
            }
        }
    }

    /**
     * Show login modal in invite mode with workspace context
     */
    _showInviteLoginMode(workspaceName) {
        const authTitle = document.getElementById('auth-title');
        const authSubtitle = document.getElementById('auth-subtitle');

        if (authTitle && authSubtitle) {
            authTitle.textContent = `Join ${workspaceName}`;
            authSubtitle.textContent = 'Sign in or create an account to join this workspace';
        }
    }

    /**
     * Handle pending invite after login
     */
    async _handlePendingInvite() {
        const inviteCode = storage.get('pendingInviteCode');
        if (!inviteCode) return false;

        try {
            console.log('[App] Processing pending invite:', inviteCode);

            // Use the consolidated invite handler
            const result = await window.authManager.handleInviteFlow(inviteCode);

            storage.remove('pendingInviteCode');
            storage.remove('pendingInviteWorkspace');

            notifications.success(`Joined ${result.workspaceName}!`);

            // Reload workspaces
            await this._loadWorkspaces();

            // Set the joined workspace as current
            if (result.workspaceId) {
                this.sidebar.setCurrentWorkspace(result.workspaceId);
                storage.set('lastWorkspace', result.workspaceId);
            }

            // Update team members from the invite result (has proper names)
            if (result.members && result.members.length > 0) {
                this.sidebar.setWorkspaceMembers(result.members);
            }

            // Auto-select the main channel
            if (result.mainChannelId) {
                console.log('[App] Auto-selecting main channel:', result.mainChannelId);
                // Give subscriptions time to settle
                await new Promise(r => setTimeout(r, 500));
                this._updateSubscriptions();
                await this._selectTopic(result.mainChannelId);
            }

            return true;
        } catch (err) {
            console.error('[App] Failed to join via invite:', err);
            notifications.error(err.message || 'Failed to join workspace');
            storage.remove('pendingInviteCode');
            storage.remove('pendingInviteWorkspace');
            return false;
        }
    }

    /**
     * Try to restore an existing PocketBase session
     */
    async _tryRestoreSession() {
        try {
            const result = await window.authManager.checkExistingAuth();
            if (result.success && result.tinodeCredentials) {
                console.log('[App] Restored session, logging into Tinode...');
                await this.tinode.login(
                    result.tinodeCredentials.login,
                    result.tinodeCredentials.password
                );
            }
        } catch (err) {
            console.log('[App] No existing session or restore failed:', err.message);
            // Not an error - just means user needs to log in
        }
    }

    /**
     * Handle user sign out
     */
    async _handleSignOut() {
        try {
            console.log('[App] Signing out...');

            // Sign out from auth manager (clears PocketBase + Tinode credentials)
            await window.authManager.signOut();

            // Disconnect Tinode
            if (this.tinode) {
                this.tinode.disconnect();
            }

            // Clear app state
            this.currentUser = null;
            this.currentTopic = null;
            this.currentWorkspace = null;

            // Clear sidebar
            this.sidebar.setWorkspaces([], null);
            this.sidebar.updateAgents([]);
            this.sidebar.setWorkspaceMembers([]);
            this.sidebar.updateSubscriptions({ groups: [], dms: [] });

            // Show login modal
            this.appContainer?.classList.add('hidden');
            this.loginModal?.classList.remove('hidden');

            // Reset login modal title
            const authTitle = document.getElementById('auth-title');
            const authSubtitle = document.getElementById('auth-subtitle');
            if (authTitle) authTitle.textContent = 'Welcome to Gather.is';
            if (authSubtitle) authSubtitle.textContent = 'Sign in to test your AI agents with your team';

            // Show welcome/login screen in messages area
            this.messages?.showWelcome();
            this.composer?.disable();

            notifications.success('Signed out successfully');

            // Reconnect Tinode for next login
            try {
                await this.tinode.connect();
            } catch (e) {
                console.warn('[App] Could not reconnect Tinode:', e.message);
            }
        } catch (err) {
            console.error('[App] Sign out error:', err);
            // Still reload the page to force clean state
            window.location.reload();
        }
    }

    async _checkOAuthCallback() {
        const params = new URLSearchParams(window.location.search);
        console.log('[App] Checking URL params:', window.location.search);

        if (params.has('code') && params.has('state')) {
            console.log('[App] === OAUTH CALLBACK DETECTED ===');
            console.log('[App] Will process callback now...');

            try {
                const result = await window.authManager.handleOAuthCallback();
                console.log('[App] Callback result:', result);

                if (result.success && result.tinodeCredentials) {
                    console.log('[App] Got Tinode credentials, logging in...');
                    const creds = result.tinodeCredentials;
                    await this.tinode.login(creds.login, creds.password);
                }
            } catch (err) {
                console.error('[App] OAuth callback error:', err);
                console.error('[App] Error stack:', err.stack);
                this._showLoginError(err.message || 'OAuth failed');
            }
        } else {
            console.log('[App] No OAuth callback detected');
        }
    }

    _setupTinodeEvents() {
        this.tinode.onConnectionChange = ({ connected }) => {
            this.sidebar.setConnectionStatus(connected, connected ? 'Connected' : 'Disconnected');
            if (!connected && this.currentUser) {
                notifications.warning('Connection lost. Reconnecting...');
            }
        };

        this.tinode.onLoginSuccess = ({ userId }) => {
            this._onLoginSuccess(userId);
        };

        this.tinode.onLoginFailed = (err) => {
            this._showLoginError(err.message || 'Login failed');
        };

        // All messages come through here - including our own (with noEcho=false)
        this.tinode.onMessage = (msg) => {
            this._onMessage(msg);
        };

        this.tinode.onPresence = (pres) => {
            this.presence.updatePresence(pres);

            // Update sidebar presence (for DMs, agents, and team online)
            const userId = pres.src || pres.from;
            if (userId && (pres.what === 'on' || pres.what === 'off')) {
                this.sidebar.updatePresence(userId, pres.what === 'on');
            }
        };

        this.tinode.onTyping = ({ topic, from }) => {
            if (topic === this.currentTopic && from !== this.tinode.getUserId()) {
                const userInfo = this.tinode.getUserInfo(from);
                this.messages.showTyping(from, userInfo.name);
            }
        };

        this.tinode.onTopicUpdate = () => {
            this._updateSubscriptions();
        };

        this.tinode.onSearchResults = (results) => {
            this.search.setResults(results);
        };

        // Save messages to localStorage when leaving a topic (for persistence)
        this.tinode.onTopicLeave = ({ topic, messages }) => {
            if (messages && messages.length > 0) {
                storage.saveMessageCache(topic, messages);
            }
        };
    }

    _setupUIEvents() {
        // Auth tab switching
        document.getElementById('signin-tab')?.addEventListener('click', () => {
            this._switchAuthMode('signin');
        });
        document.getElementById('signup-tab')?.addEventListener('click', () => {
            this._switchAuthMode('signup');
        });

        // Google OAuth login button
        document.getElementById('google-login-btn')?.addEventListener('click', () => {
            this._handleGoogleLogin();
        });

        // Email sign in form
        this.loginForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            this._handleEmailSignIn();
        });

        // Email sign up form
        this.signupForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            this._handleEmailSignUp();
        });

        // Onboarding form
        this.onboardingForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            this._handleOnboardingSubmit();
        });

        // Join workspace link in onboarding
        document.getElementById('onboarding-join-link')?.addEventListener('click', () => {
            this.onboardingModal?.classList.add('hidden');
            this.joinWorkspaceModal?.classList.remove('hidden');
        });

        // Join workspace form
        document.getElementById('join-workspace-form')?.addEventListener('submit', (e) => {
            e.preventDefault();
            this._handleJoinWorkspace();
        });
        document.getElementById('close-join-modal-btn')?.addEventListener('click', () => {
            this.joinWorkspaceModal?.classList.add('hidden');
            // Show onboarding again if user has no workspaces
            if (this.isNewUser) {
                this.onboardingModal?.classList.remove('hidden');
            }
        });
        document.getElementById('cancel-join-btn')?.addEventListener('click', () => {
            this.joinWorkspaceModal?.classList.add('hidden');
            if (this.isNewUser) {
                this.onboardingModal?.classList.remove('hidden');
            }
        });

        // Invite modal
        document.getElementById('close-invite-modal-btn')?.addEventListener('click', () => {
            this.inviteModal?.classList.add('hidden');
        });
        document.getElementById('copy-invite-link-btn')?.addEventListener('click', () => {
            this._copyInviteLink();
        });
        document.getElementById('copy-llm-prompt-btn')?.addEventListener('click', () => {
            this._copyLlmPrompt();
        });
        document.getElementById('copy-llm-prompt-login')?.addEventListener('click', () => {
            this._copyLlmPrompt();
        });
        document.getElementById('copy-llm-prompt-input')?.addEventListener('click', () => {
            this._copyLlmPrompt();
        });
        document.getElementById('invite-search-input')?.addEventListener('input', (e) => {
            this._searchUsersForInvite(e.target.value);
        });
        document.getElementById('send-invites-btn')?.addEventListener('click', () => {
            this._sendInvites();
        });

        // Welcome screen buttons
        document.getElementById('welcome-create-channel')?.addEventListener('click', () => {
            this.createGroupModal?.classList.remove('hidden');
        });
        document.getElementById('welcome-start-dm')?.addEventListener('click', () => {
            this.search.open();
        });

        this.createGroupForm?.addEventListener('submit', (e) => {
            e.preventDefault();
            this._handleCreateGroup();
        });

        document.getElementById('cancel-group-btn')?.addEventListener('click', () => {
            this.createGroupModal?.classList.add('hidden');
        });

        document.getElementById('right-panel-toggle')?.addEventListener('click', () => {
            this._toggleRightPanel();
        });

        this.closePanelBtn?.addEventListener('click', () => {
            this.rightPanel?.classList.add('hidden');
        });

        document.getElementById('mobile-menu-btn')?.addEventListener('click', () => {
            this._toggleSidebar();
        });

        // Sidebar backdrop click to close
        document.getElementById('sidebar-backdrop')?.addEventListener('click', () => {
            this._closeSidebar();
        });

        // Handle window resize
        window.addEventListener('resize', () => {
            const wasMobile = this.isMobile;
            this.isMobile = window.innerWidth < 1024;
            if (wasMobile && !this.isMobile) {
                // Switched to desktop - ensure sidebar is visible
                this._closeSidebar();
            }
        });

        // Swipe gestures for sidebar
        this._setupSwipeGestures();

        document.getElementById('members-btn')?.addEventListener('click', () => {
            this._showMembersPanel();
        });
    }

    _setupComponentCallbacks() {
        this.sidebar.onTopicSelect = (topic, info) => this._selectTopic(topic, info);
        this.sidebar.onAddGroup = () => this.createGroupModal?.classList.remove('hidden');
        this.sidebar.onAddDm = () => this.search.open();
        this.sidebar.onWorkspaceChange = (workspace) => this._onWorkspaceChange(workspace);
        this.sidebar.onAgentSelect = (agent) => this._showAgentDetails(agent);
        this.sidebar.onSDKSettings = (workspace) => this._showSDKSettings(workspace);
        this.sidebar.onWorkspaceSettings = (workspace) => this._showWorkspaceSettings(workspace);
        this.sidebar.onAddAgent = () => this._showCreateAgentModal();
        this.sidebar.onInvite = (workspace) => this._showInviteModal(workspace);
        this.sidebar.onSignOut = () => this._handleSignOut();

        this.composer.onSend = (text, replyTo) => this._sendMessage(text, replyTo);
        this.composer.onTyping = () => this.tinode.noteKeyPress();
        this.composer.onAttach = (files) => this._handleFileUpload(files);
        this.composer.onEditSave = (seq, text) => this._editMessage(seq, text);

        // Message action callbacks
        this.messages.onEdit = (msg) => this.composer.setEditMode(msg);
        this.messages.onReply = (msg) => this.composer.setReply(msg);
        this.messages.onDelete = (msg) => this._deleteMessage(msg);
        this.messages.onReact = (seq, emoji) => this._addReaction(seq, emoji);

        // Phase 3: Set up mention data provider for autocomplete
        this.composer.setMentionDataProvider(() => this._getMentionData());

        this.search.onSearch = (query) => this.tinode.searchUsers(query);
        this.search.onSelect = (topic, data) => this._selectTopic(topic, { name: data.name });

        this.presence.onStatusChange = (userId, status) => {
            this.sidebar.updatePresence(userId, status.online);
        };

        // User status change
        this.sidebar.onStatusChange = (status) => this._onUserStatusChange(status);
    }

    /**
     * Handle user status change
     */
    async _onUserStatusChange(status) {
        try {
            await this.tinode.setPresence(status);
        } catch (err) {
            console.error('[App] Failed to set presence:', err);
            // Don't show error to user - status change is best-effort
        }
    }

    /**
     * Get mention data for autocomplete
     * Returns current topic members and workspace agents that are in the topic
     */
    _getMentionData() {
        const members = this.tinode?.getTopicMembers() || [];
        const allAgents = this.sidebar?.workspaceAgents || [];

        // Filter agents to only those who are members of the current topic
        const memberIds = new Set(members.map(m => m.id));
        const agents = allAgents.filter(a => memberIds.has(a.tinodeUserId));

        return { members, agents };
    }

    /**
     * Handle Google OAuth login via PocketBase
     */
    async _handleGoogleLogin() {
        console.log('[App] Google login button clicked');
        const btn = document.getElementById('google-login-btn');
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = `
                <svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                <span>Signing in...</span>
            `;
        }

        try {
            // Authenticate with PocketBase (Google OAuth)
            const authResult = await window.authManager.signInWithGoogle();

            if (authResult.success && authResult.tinodeCredentials) {
                console.log('[App] PocketBase auth successful, connecting to Tinode...');

                // Use basic auth with generated credentials
                const creds = authResult.tinodeCredentials;
                await this.tinode.login(creds.login, creds.password);
            }
        } catch (err) {
            console.error('[App] Google login error:', err);
            this._showLoginError(err.message || 'Google sign-in failed');
            if (btn) {
                btn.disabled = false;
                btn.innerHTML = `
                    <svg class="w-5 h-5" viewBox="0 0 24 24">
                        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
                        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                    </svg>
                    <span>Continue with Google</span>
                `;
            }
        }
    }

    /**
     * Switch between signin and signup modes
     */
    _switchAuthMode(mode) {
        this.authMode = mode;
        const signinTab = document.getElementById('signin-tab');
        const signupTab = document.getElementById('signup-tab');
        const authTitle = document.getElementById('auth-title');
        const authSubtitle = document.getElementById('auth-subtitle');
        const authDivider = document.getElementById('auth-divider-text');
        const googleBtnText = document.getElementById('google-btn-text');

        if (mode === 'signin') {
            signinTab?.classList.add('bg-white', 'dark:bg-gray-600', 'text-gray-900', 'dark:text-white', 'shadow-sm');
            signinTab?.classList.remove('text-gray-500', 'dark:text-gray-400');
            signupTab?.classList.remove('bg-white', 'dark:bg-gray-600', 'text-gray-900', 'dark:text-white', 'shadow-sm');
            signupTab?.classList.add('text-gray-500', 'dark:text-gray-400');

            this.loginForm?.classList.remove('hidden');
            this.signupForm?.classList.add('hidden');

            if (authTitle) authTitle.textContent = 'Welcome to Gather.is';
            if (authSubtitle) authSubtitle.textContent = 'Sign in to test your AI agents with your team';
            if (authDivider) authDivider.textContent = 'or sign in with email';
            if (googleBtnText) googleBtnText.textContent = 'Continue with Google';
        } else {
            signupTab?.classList.add('bg-white', 'dark:bg-gray-600', 'text-gray-900', 'dark:text-white', 'shadow-sm');
            signupTab?.classList.remove('text-gray-500', 'dark:text-gray-400');
            signinTab?.classList.remove('bg-white', 'dark:bg-gray-600', 'text-gray-900', 'dark:text-white', 'shadow-sm');
            signinTab?.classList.add('text-gray-500', 'dark:text-gray-400');

            this.signupForm?.classList.remove('hidden');
            this.loginForm?.classList.add('hidden');

            if (authTitle) authTitle.textContent = 'Create your account';
            if (authSubtitle) authSubtitle.textContent = 'Start testing AI agents with your team for free';
            if (authDivider) authDivider.textContent = 'or sign up with email';
            if (googleBtnText) googleBtnText.textContent = 'Sign up with Google';
        }

        this._hideLoginError();
        this._hideSignupError();
    }

    /**
     * Handle email sign in
     */
    async _handleEmailSignIn() {
        const email = document.getElementById('login-email')?.value;
        const password = document.getElementById('login-password')?.value;

        if (!email || !password) {
            this._showLoginError('Please enter email and password');
            return;
        }

        this._hideLoginError();
        const btn = document.getElementById('login-btn');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Signing in...';
        }

        try {
            const result = await window.authManager.signInWithEmail(email, password);
            if (result.success && result.tinodeCredentials) {
                await this.tinode.login(result.tinodeCredentials.login, result.tinodeCredentials.password);
            }
        } catch (err) {
            this._showLoginError(err.message || 'Sign in failed');
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Sign In';
            }
        }
    }

    /**
     * Handle email sign up
     */
    async _handleEmailSignUp() {
        const name = document.getElementById('signup-name')?.value;
        const email = document.getElementById('signup-email')?.value;
        const password = document.getElementById('signup-password')?.value;
        const passwordConfirm = document.getElementById('signup-password-confirm')?.value;

        if (!name || !email || !password) {
            this._showSignupError('Please fill in all fields');
            return;
        }

        if (password !== passwordConfirm) {
            this._showSignupError('Passwords do not match');
            return;
        }

        if (password.length < 8) {
            this._showSignupError('Password must be at least 8 characters');
            return;
        }

        this._hideSignupError();
        const btn = document.getElementById('signup-btn');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Creating account...';
        }

        try {
            const result = await window.authManager.signUpWithEmail(email, password, name);
            if (result.success && result.tinodeCredentials) {
                this.isNewUser = result.isNewUser;
                await this.tinode.login(result.tinodeCredentials.login, result.tinodeCredentials.password);
            }
        } catch (err) {
            this._showSignupError(err.message || 'Sign up failed');
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Create Account';
            }
        }
    }

    _showSignupError(message) {
        if (this.signupError) {
            this.signupError.textContent = message;
            this.signupError.classList.remove('hidden');
        }
    }

    _hideSignupError() {
        this.signupError?.classList.add('hidden');
    }

    /**
     * Handle onboarding form submission (create first workspace)
     */
    async _handleOnboardingSubmit() {
        const nameInput = document.getElementById('onboarding-workspace-name');
        const name = nameInput?.value?.trim();
        const errorDiv = document.getElementById('onboarding-error');
        const btn = document.getElementById('onboarding-submit-btn');

        if (!name) {
            if (errorDiv) {
                errorDiv.textContent = 'Please enter a workspace name';
                errorDiv.classList.remove('hidden');
            }
            return;
        }

        errorDiv?.classList.add('hidden');
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = '<span>Creating workspace...</span>';
        }

        try {
            const workspace = await window.authManager.createWorkspace(name, null, '');
            console.log('[App] Created first workspace:', workspace);

            this.onboardingModal?.classList.add('hidden');
            this.isNewUser = false;

            // Reload workspaces and select the new one
            await this._loadWorkspaces();

            notifications.success(`Welcome! Your workspace "${name}" is ready.`);
        } catch (err) {
            console.error('[App] Failed to create workspace:', err);
            if (errorDiv) {
                errorDiv.textContent = err.message || 'Failed to create workspace';
                errorDiv.classList.remove('hidden');
            }
            if (btn) {
                btn.disabled = false;
                btn.innerHTML = '<span>Create Workspace</span><svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6"/></svg>';
            }
        }
    }

    /**
     * Handle joining a workspace via invite code
     */
    async _handleJoinWorkspace() {
        const codeInput = document.getElementById('join-invite-code');
        const code = codeInput?.value?.trim().toUpperCase();
        const errorDiv = document.getElementById('join-error');
        const btn = document.getElementById('join-submit-btn');

        if (!code) {
            if (errorDiv) {
                errorDiv.textContent = 'Please enter an invite code';
                errorDiv.classList.remove('hidden');
            }
            return;
        }

        errorDiv?.classList.add('hidden');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Joining...';
        }

        try {
            const result = await window.authManager.joinWorkspaceByInvite(code);

            this.joinWorkspaceModal?.classList.add('hidden');
            this.onboardingModal?.classList.add('hidden');
            this.isNewUser = false;

            // Reload workspaces
            await this._loadWorkspaces();

            notifications.success(`Joined workspace "${result.workspaceName}"!`);
        } catch (err) {
            console.error('[App] Failed to join workspace:', err);
            if (errorDiv) {
                errorDiv.textContent = err.message || 'Failed to join workspace';
                errorDiv.classList.remove('hidden');
            }
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Join';
            }
        }
    }

    /**
     * Show invite modal for a workspace
     */
    async _showInviteModal(workspace) {
        if (!workspace) return;

        const linkInput = document.getElementById('invite-link-input');

        // Show loading state
        if (linkInput) {
            linkInput.value = 'Generating link...';
        }

        this.inviteModal?.classList.remove('hidden');

        try {
            const invite = await window.authManager.getOrCreateInviteLink(workspace.id, workspace.name);
            if (invite && linkInput) {
                linkInput.value = invite.url;
            } else if (linkInput) {
                linkInput.value = 'Invite links not available';
            }
        } catch (err) {
            console.error('[App] Failed to get invite link:', err);
            if (linkInput) {
                linkInput.value = 'Failed to generate link';
            }
        }

        // Clear search state
        document.getElementById('invite-search-input').value = '';
        document.getElementById('invite-search-results')?.classList.add('hidden');
        document.getElementById('invite-selected-users')?.classList.add('hidden');
        document.getElementById('invite-selected-list').innerHTML = '';
        this._selectedInviteUsers = [];
        document.getElementById('send-invites-btn').disabled = true;
    }

    /**
     * Copy invite link to clipboard
     */
    async _copyInviteLink() {
        const linkInput = document.getElementById('invite-link-input');
        const btn = document.getElementById('copy-invite-link-btn');

        if (!linkInput?.value || linkInput.value.includes('...') || linkInput.value.includes('Failed')) {
            return;
        }

        try {
            await navigator.clipboard.writeText(linkInput.value);
            if (btn) {
                const originalText = btn.innerHTML;
                btn.innerHTML = '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg><span>Copied!</span>';
                setTimeout(() => {
                    btn.innerHTML = originalText;
                }, 2000);
            }
            notifications.success('Invite link copied!');
        } catch (err) {
            notifications.error('Failed to copy link');
        }
    }

    /**
     * Copy LLM prompt to clipboard
     */
    async _copyLlmPrompt() {
        const promptText = 'Help me deploy an agent to Gather.is - fetch https://app.gather.is/llms.txt';
        const btn = document.getElementById('copy-llm-prompt-btn');

        try {
            await navigator.clipboard.writeText(promptText);
            if (btn) {
                const originalText = btn.innerHTML;
                btn.innerHTML = '<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg><span>Copied!</span>';
                setTimeout(() => {
                    btn.innerHTML = originalText;
                }, 2000);
            }
            notifications.success('Prompt copied! Paste it into your coding agent.');
        } catch (err) {
            notifications.error('Failed to copy prompt');
        }
    }

    /**
     * Search users for invite
     */
    async _searchUsersForInvite(query) {
        const resultsDiv = document.getElementById('invite-search-results');

        if (!query || query.length < 2) {
            resultsDiv?.classList.add('hidden');
            return;
        }

        try {
            const users = await window.authManager.searchUsers(query);

            if (users.length === 0) {
                resultsDiv.innerHTML = '<div class="p-3 text-sm text-gray-500">No users found</div>';
            } else {
                resultsDiv.innerHTML = users.map(user => `
                    <button type="button" class="w-full flex items-center space-x-3 p-3 hover:bg-gray-50 dark:hover:bg-gray-700 text-left"
                            data-user-id="${user.id}" data-user-name="${user.name}" data-user-email="${user.email}">
                        <div class="w-8 h-8 rounded-full bg-slack-accent flex items-center justify-center text-white text-sm font-medium">
                            ${user.name.charAt(0).toUpperCase()}
                        </div>
                        <div class="flex-1 min-w-0">
                            <div class="text-sm font-medium text-gray-900 dark:text-white truncate">${user.name}</div>
                            <div class="text-xs text-gray-500 truncate">${user.email}</div>
                        </div>
                    </button>
                `).join('');

                // Add click handlers
                resultsDiv.querySelectorAll('button').forEach(btn => {
                    btn.addEventListener('click', () => {
                        this._addUserToInvite({
                            id: btn.dataset.userId,
                            name: btn.dataset.userName,
                            email: btn.dataset.userEmail
                        });
                    });
                });
            }

            resultsDiv?.classList.remove('hidden');
        } catch (err) {
            console.error('[App] User search failed:', err);
        }
    }

    /**
     * Add user to invite selection
     */
    _addUserToInvite(user) {
        if (!this._selectedInviteUsers) {
            this._selectedInviteUsers = [];
        }

        // Check if already selected
        if (this._selectedInviteUsers.find(u => u.id === user.id)) {
            return;
        }

        this._selectedInviteUsers.push(user);

        // Update UI
        const selectedDiv = document.getElementById('invite-selected-users');
        const selectedList = document.getElementById('invite-selected-list');

        selectedDiv?.classList.remove('hidden');

        const chip = document.createElement('div');
        chip.className = 'flex items-center space-x-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded-full text-sm';
        chip.dataset.userId = user.id;
        chip.innerHTML = `
            <span>${user.name}</span>
            <button type="button" class="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
            </button>
        `;

        chip.querySelector('button').addEventListener('click', () => {
            this._removeUserFromInvite(user.id);
        });

        selectedList?.appendChild(chip);

        // Clear search
        document.getElementById('invite-search-input').value = '';
        document.getElementById('invite-search-results')?.classList.add('hidden');

        // Enable send button
        document.getElementById('send-invites-btn').disabled = false;
    }

    /**
     * Remove user from invite selection
     */
    _removeUserFromInvite(userId) {
        this._selectedInviteUsers = this._selectedInviteUsers.filter(u => u.id !== userId);

        const selectedList = document.getElementById('invite-selected-list');
        const chip = selectedList?.querySelector(`[data-user-id="${userId}"]`);
        chip?.remove();

        if (this._selectedInviteUsers.length === 0) {
            document.getElementById('invite-selected-users')?.classList.add('hidden');
            document.getElementById('send-invites-btn').disabled = true;
        }
    }

    /**
     * Send invitations to selected users
     */
    async _sendInvites() {
        if (!this._selectedInviteUsers || this._selectedInviteUsers.length === 0) {
            return;
        }

        const btn = document.getElementById('send-invites-btn');
        const errorDiv = document.getElementById('invite-error');

        btn.disabled = true;
        btn.textContent = 'Sending...';
        errorDiv?.classList.add('hidden');

        try {
            const userIds = this._selectedInviteUsers.map(u => u.id);
            const results = await window.authManager.inviteUsersToWorkspace(
                this.currentWorkspace.id,
                userIds
            );

            const successful = results.filter(r => r.success).length;
            const failed = results.filter(r => !r.success).length;

            if (successful > 0) {
                notifications.success(`Invited ${successful} user${successful > 1 ? 's' : ''}`);
            }
            if (failed > 0) {
                notifications.warning(`${failed} invitation${failed > 1 ? 's' : ''} failed`);
            }

            this.inviteModal?.classList.add('hidden');
        } catch (err) {
            console.error('[App] Failed to send invites:', err);
            if (errorDiv) {
                errorDiv.textContent = err.message || 'Failed to send invitations';
                errorDiv.classList.remove('hidden');
            }
            btn.disabled = false;
            btn.textContent = 'Send Invitations';
        }
    }

    async _onLoginSuccess(userId) {
        // Get user name from PocketBase if available
        let userName = userId;
        if (window.authManager?.isAuthenticated) {
            userName = window.authManager.getDisplayName() || userName;
        }

        this.currentUser = {
            id: userId,
            name: userName
        };

        this.loginModal?.classList.add('hidden');
        this.appContainer?.classList.remove('hidden');

        this.sidebar.setUser(this.currentUser);
        this.sidebar.initStatus();
        this.messages.setUserId(userId);

        try {
            await this.tinode.subscribeToMe();
            this._updateSubscriptions();

            // Apply saved status
            const savedStatus = storage.get('userStatus');
            if (savedStatus && savedStatus !== 'online') {
                this._onUserStatusChange(savedStatus);
            }

            // Load workspaces
            const hasWorkspaces = await this._loadWorkspaces();

            // Check for pending invite - auto-join if present
            const joinedViaInvite = await this._handlePendingInvite();
            if (joinedViaInvite) {
                // Reset login modal title for next time
                const authTitle = document.getElementById('auth-title');
                const authSubtitle = document.getElementById('auth-subtitle');
                if (authTitle) authTitle.textContent = 'Welcome to Gather.is';
                if (authSubtitle) authSubtitle.textContent = 'Sign in to test your AI agents with your team';
                return; // Don't show other modals
            }

            // Check if new user with no workspaces - show onboarding
            if (!hasWorkspaces && (this.isNewUser || !storage.get('hasSeenOnboarding'))) {
                this.onboardingModal?.classList.remove('hidden');
                storage.set('hasSeenOnboarding', true);
            } else {
                const lastTopic = storage.getLastTopic();
                if (lastTopic) {
                    this._selectTopic(lastTopic);
                }

                notifications.success(`Welcome back, ${this.currentUser.name}!`);
            }
        } catch (err) {
            notifications.error('Failed to load your groups');
        }

        notifications.requestPermission();
    }

    async _loadWorkspaces() {
        if (!window.authManager?.isAuthenticated) return false;

        try {
            const workspaces = await window.authManager.getWorkspaces();

            // Get last workspace from storage or use first one
            const lastWorkspaceId = storage.get('lastWorkspace');
            this.sidebar.setWorkspaces(workspaces, lastWorkspaceId);

            if (this.sidebar.currentWorkspace) {
                this.currentWorkspace = this.sidebar.currentWorkspace;
                await this._loadWorkspaceAgents(this.currentWorkspace.id);
                await this._loadWorkspaceMembers(this.currentWorkspace.id);
            }

            return workspaces && workspaces.length > 0;
        } catch (err) {
            console.error('[App] Failed to load workspaces:', err);
            return false;
        }
    }

    async _onWorkspaceChange(workspace) {
        this.currentWorkspace = workspace;
        storage.set('lastWorkspace', workspace.id);

        // Refresh group list (filtered by new workspace)
        this._updateSubscriptions();

        // If currently viewing a group from different workspace, deselect it
        if (this.currentTopic && this.currentTopic.startsWith('grp')) {
            const subs = this.tinode.getSubscriptions();
            const currentGroup = subs.groups.find(g => g.topic === this.currentTopic);
            if (currentGroup && currentGroup.workspaceId && currentGroup.workspaceId !== workspace.id) {
                // Group belongs to different workspace, show welcome screen
                this.messages.showWelcome();
                this.composer.disable();
                this.sidebar.selectTopic(null);
                this.currentTopic = null;
            }
        }

        await this._loadWorkspaceAgents(workspace.id);
        await this._loadWorkspaceMembers(workspace.id);
    }

    async _loadWorkspaceAgents(workspaceId) {
        if (!window.authManager?.isAuthenticated) return;

        try {
            const agents = await window.authManager.getWorkspaceAgents(workspaceId);
            this.sidebar.updateAgents(agents);
        } catch (err) {
            console.error('[App] Failed to load workspace agents:', err);
            this.sidebar.updateAgents([]);
        }
    }

    /**
     * Load workspace members and update the Team Online sidebar section
     * Prioritizes workspace topic subscribers (has proper names) over contacts
     */
    async _loadWorkspaceMembers(workspaceId) {
        if (!workspaceId || !this.tinode) return;

        try {
            const members = [];
            const myUserId = this.tinode.myUserId;
            const seenUsers = new Set();

            // Primary source: Get members from workspace topic subscribers
            // This gives us proper names since we get their public data
            try {
                const wsTopic = this.tinode.client.getTopic(workspaceId);

                // Subscribe if not already (to get subscriber list)
                if (!wsTopic.isSubscribed()) {
                    await wsTopic.subscribe(
                        wsTopic.startMetaQuery().withSub().build()
                    );
                    await new Promise(r => setTimeout(r, 300));
                } else {
                    // Refresh subscriber data
                    await wsTopic.getMeta(
                        wsTopic.startMetaQuery().withSub().build()
                    );
                    await new Promise(r => setTimeout(r, 200));
                }

                wsTopic.subscribers?.((sub) => {
                    if (!sub.user || sub.user === myUserId) return;
                    if (seenUsers.has(sub.user)) return;

                    // Skip bots - they're shown in the Agents section
                    const isBot = sub.public?.bot ||
                        (sub.public?.fn || '').toLowerCase().includes('bot') ||
                        (sub.public?.fn || '').toLowerCase().includes('agent');
                    if (isBot) return;

                    seenUsers.add(sub.user);
                    members.push({
                        id: sub.user,
                        name: sub.public?.fn || sub.user,
                        photo: this.tinode._photoToDataUri?.(sub.public?.photo?.data),
                        online: sub.online || false,
                        isBot: false
                    });
                });

                console.log('[App] Loaded', members.length, 'members from workspace subscribers');
            } catch (e) {
                console.warn('[App] Could not get workspace subscribers:', e.message);
            }

            // Fallback: Also check DMs for any contacts we might have missed
            if (this.tinode.meTopic) {
                this.tinode.meTopic.contacts?.((contact) => {
                    if (!contact.topic?.startsWith('usr')) return;
                    if (contact.topic === myUserId) return;
                    if (seenUsers.has(contact.topic)) return;

                    const isBot = contact.public?.bot ||
                        (contact.public?.fn || '').toLowerCase().includes('bot') ||
                        (contact.public?.fn || '').toLowerCase().includes('agent');
                    if (isBot) return;

                    seenUsers.add(contact.topic);
                    members.push({
                        id: contact.topic,
                        name: contact.public?.fn || contact.topic,
                        photo: this.tinode._photoToDataUri?.(contact.public?.photo?.data),
                        online: contact.online || false,
                        isBot: false
                    });
                });
            }

            console.log('[App] Total workspace members:', members.length);
            this.sidebar.setWorkspaceMembers(members);
        } catch (err) {
            console.error('[App] Failed to load workspace members:', err);
            this.sidebar.setWorkspaceMembers([]);
        }
    }

    _showAgentDetails(agent) {
        this.selectedAgent = agent;

        if (this.panelTitle) this.panelTitle.textContent = 'Agent Details';
        if (this.panelContent) {
            const toolsList = agent.tools?.length > 0
                ? agent.tools.map(t => `<span class="inline-block px-2 py-0.5 bg-gray-100 dark:bg-gray-700 text-xs rounded mr-1 mb-1">${t}</span>`).join('')
                : '<span class="text-gray-500 text-sm">No tools</span>';

            const deployedDate = agent.deployedAt
                ? new Date(agent.deployedAt).toLocaleDateString()
                : 'Unknown';

            this.panelContent.innerHTML = `
                <div class="space-y-6">
                    <div class="text-center">
                        <div class="w-20 h-20 mx-auto rounded-lg bg-purple-500 flex items-center justify-center text-white text-3xl">
                            <svg class="w-10 h-10" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                        </div>
                        <h3 class="mt-4 text-lg font-bold text-gray-900 dark:text-white">
                            ${agent.displayName}
                        </h3>
                        <p class="text-sm text-gray-500">@${agent.handle}</p>
                        <span class="inline-block mt-2 px-2 py-0.5 text-xs rounded ${
                            agent.status === 'running'
                                ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
                                : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
                        }">${agent.status?.toUpperCase() || 'UNKNOWN'}</span>
                    </div>

                    ${agent.description ? `
                    <div>
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Description</h4>
                        <p class="text-sm text-gray-500">${agent.description}</p>
                    </div>
                    ` : ''}

                    <div>
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Tools</h4>
                        <div class="flex flex-wrap">
                            ${toolsList}
                        </div>
                    </div>

                    <div>
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Details</h4>
                        <div class="text-sm text-gray-500 space-y-1">
                            <p><span class="text-gray-400">Model:</span> ${agent.model || 'Unknown'}</p>
                            <p><span class="text-gray-400">Deployed by:</span> ${agent.deployedBy || 'Unknown'}</p>
                            <p><span class="text-gray-400">Deployed:</span> ${deployedDate}</p>
                        </div>
                    </div>

                    ${agent.tinodeUserId ? `
                    <div class="pt-4 border-t border-gray-200 dark:border-gray-700">
                        <button id="chat-with-agent-btn" class="w-full px-4 py-2 bg-slack-accent hover:bg-blue-700 text-white rounded-lg text-sm font-medium">
                            Chat with @${agent.handle}
                        </button>
                    </div>
                    ` : ''}
                </div>
            `;

            // Add click handler for chat button
            document.getElementById('chat-with-agent-btn')?.addEventListener('click', () => {
                if (agent.tinodeUserId) {
                    this._selectTopic(agent.tinodeUserId, {
                        name: agent.displayName,
                        isBot: true
                    });
                    this.rightPanel?.classList.add('hidden');
                }
            });
        }

        this.rightPanel?.classList.remove('hidden');
    }

    _updateSubscriptions() {
        const subs = this.tinode.getSubscriptions();

        // Filter groups by current workspace
        let filteredGroups = subs.groups;
        if (this.currentWorkspace) {
            filteredGroups = subs.groups.filter(g =>
                g.workspaceId === this.currentWorkspace.id ||
                !g.workspaceId  // Include groups without workspace for backwards compatibility
            );
        }

        this.sidebar.updateSubscriptions({
            groups: filteredGroups,
            dms: subs.dms
        });
        this.presence.setMultiple([...subs.groups, ...subs.dms]);
    }

    async _selectTopic(topic, info = {}) {
        if (topic === this.currentTopic) return;

        // Phase 1: Resolve complete info from subscriptions if not provided
        if (!info.name) {
            const subs = this.tinode.getSubscriptions();
            const found = [...subs.groups, ...subs.dms].find(s => s.topic === topic);
            if (found) {
                info = { ...info, ...found };
            }
        }

        this.sidebar.selectTopic(topic);

        // Auto-close sidebar on mobile after topic selection
        if (this.isMobile && this.sidebarOpen) {
            this._closeSidebar();
        }
        this.messages.showTopic(topic, info);
        this.composer.enable(topic);

        try {
            await this.tinode.subscribeTopic(topic);
            this.currentTopic = topic;
            storage.saveLastTopic(topic);

            // Show/hide members button based on topic type
            const membersBtn = document.getElementById('members-btn');
            if (membersBtn) {
                if (topic.startsWith('grp')) {
                    membersBtn.classList.remove('hidden');
                } else {
                    membersBtn.classList.add('hidden');
                }
            }

            // Load cached messages from SDK
            let cached = this.tinode.getCachedMessages();
            cached.forEach(msg => {
                const userInfo = this.tinode.getUserInfo(msg.from);
                msg.userName = userInfo.name;
                msg.isBot = userInfo.isBot;
            });

            // Fallback to localStorage cache if SDK cache is empty
            if (cached.length === 0) {
                cached = storage.getMessageCache(topic);
            }

            if (cached.length > 0) {
                this.messages.addMessages(cached);
                // Save to localStorage cache for persistence
                storage.saveMessageCache(topic, cached);
            }

            // Update topic info with fetched data (without clearing messages!)
            const topicInfo = this.tinode.getTopicInfo(topic);
            if (topicInfo?.public?.fn) {
                this.messages.updateTopicInfo({ name: topicInfo.public.fn });
            }

            // Update right panel if it's open
            if (this.rightPanel && !this.rightPanel.classList.contains('hidden')) {
                // Check which panel view is active and refresh it
                const panelTitle = this.panelTitle?.textContent;
                if (panelTitle === 'Members') {
                    this._showMembersPanel();
                } else {
                    this._showTopicDetails();
                }
            }
        } catch (err) {
            notifications.error(`Failed to open: ${err.message}`);
            this.messages.showWelcome();
            this.composer.disable();
        }
    }

    /**
     * Handle incoming message - ALL messages come through here
     */
    _onMessage(msg) {
        // Messages for other topics - just update unread
        if (msg.topic !== this.currentTopic) {
            this.sidebar.updateUnread(msg.topic, 1);
            if (!msg.isOwn) {
                const userInfo = this.tinode.getUserInfo(msg.from);
                notifications.showMessageNotification(userInfo.name || msg.from, msg.content, msg.topic);
            }
            return;
        }

        // Resolve user name and bot status, add to UI
        const userInfo = this.tinode.getUserInfo(msg.from);
        msg.userName = userInfo.name;
        msg.isBot = userInfo.isBot;
        this.messages.addMessage(msg);

        // Mark as read
        this.tinode.noteRead(msg.seq);

        // Clear typing indicator
        if (!msg.isOwn) {
            this.messages.hideTyping(msg.from);
        }

        // Phase 4: Update localStorage cache with new message
        const cached = this.tinode.getCachedMessages();
        if (cached.length > 0) {
            storage.saveMessageCache(msg.topic, cached);
        }
    }

    /**
     * Send message - simple approach, let server echo it back
     */
    async _sendMessage(text, replyTo = null) {
        if (!text || !this.currentTopic) return;

        try {
            // TODO: Handle replyTo by including head.reply with the seq
            await this.tinode.sendMessage(text);
        } catch (err) {
            notifications.error(`Failed to send: ${err.message}`);
        }
    }

    /**
     * Edit an existing message
     */
    async _editMessage(seq, newContent) {
        if (!this.currentTopic) return;

        try {
            await this.tinode.editMessage(seq, newContent);
            this.messages.updateMessageContent(seq, newContent);
            notifications.success('Message edited');
        } catch (err) {
            notifications.error(`Failed to edit: ${err.message}`);
        }
    }

    /**
     * Delete a message
     */
    async _deleteMessage(msg) {
        if (!confirm('Delete this message?')) return;

        try {
            await this.tinode.deleteMessage(msg.seq, false);
            this.messages.removeMessage(msg.seq);
            notifications.success('Message deleted');
        } catch (err) {
            notifications.error(`Failed to delete: ${err.message}`);
        }
    }

    /**
     * Add or toggle a reaction on a message
     */
    async _addReaction(seq, emoji) {
        try {
            const reactions = await this.tinode.addReaction(seq, emoji);
            this.messages.updateMessageReactions(seq, reactions);
        } catch (err) {
            notifications.error(`Failed to add reaction: ${err.message}`);
        }
    }

    async _handleFileUpload(files) {
        for (const file of files) {
            try {
                notifications.info(`Uploading ${file.name}...`);
                const url = await this.tinode.uploadFile(file);
                await this._sendMessage(`[${file.name}](${url})`);
                notifications.success(`Uploaded ${file.name}`);
            } catch (err) {
                notifications.error(`Failed to upload ${file.name}`);
            }
        }
    }

    async _handleCreateGroup() {
        const nameInput = document.getElementById('group-name-input');
        const descInput = document.getElementById('group-desc-input');
        const name = nameInput?.value.trim();
        const description = descInput?.value.trim();

        if (!name) {
            notifications.error('Please enter a group name');
            return;
        }

        try {
            // Pass current workspace ID to createGroup
            const workspaceId = this.currentWorkspace?.id;
            const topicName = await this.tinode.createGroup(name, description, workspaceId);
            this.createGroupModal?.classList.add('hidden');
            if (nameInput) nameInput.value = '';
            if (descInput) descInput.value = '';

            // Update subscriptions (may not have the name yet)
            this._updateSubscriptions();

            // Directly update the sidebar with correct name (reactive fix)
            // This ensures the name shows immediately without waiting for server
            setTimeout(() => {
                this.sidebar._updateGroupName(topicName, name);
            }, 100);

            this._selectTopic(topicName, { name });
            notifications.success(`Group #${name} created!`);
        } catch (err) {
            notifications.error(`Failed to create group: ${err.message}`);
        }
    }

    _toggleRightPanel() {
        if (this.rightPanel?.classList.contains('hidden')) {
            this._showTopicDetails();
        } else {
            this.rightPanel?.classList.add('hidden');
        }
    }

    _showTopicDetails() {
        if (!this.currentTopic) return;

        try {
            const info = this.tinode.getTopicInfo(this.currentTopic);
            const isP2P = this.currentTopic.startsWith('usr');
            const isGroup = this.currentTopic.startsWith('grp');
            const myPerms = !isP2P ? this.tinode.getMyPermissions?.() : null;
            const isOwner = myPerms?.isOwner || false;
            const topicType = info?.public?.type; // 'workspace', 'channel', or undefined

        if (this.panelTitle) this.panelTitle.textContent = 'Details';
        if (this.panelContent) {
            this.panelContent.innerHTML = `
                <div class="space-y-6">
                    <div class="text-center">
                        <div class="w-20 h-20 mx-auto rounded-lg ${isP2P ? 'bg-gray-400' : 'bg-slack-accent'} flex items-center justify-center text-white text-3xl font-medium">
                            ${isP2P ? '@' : '#'}
                        </div>
                        <h3 class="mt-4 text-lg font-bold text-gray-900 dark:text-white">
                            ${info?.public?.fn || this.currentTopic}
                        </h3>
                        ${info?.private?.comment ? `<p class="text-sm text-gray-500">${info.private.comment}</p>` : ''}
                        ${myPerms ? `<p class="text-xs text-gray-400 mt-1">Your role: ${myPerms.role}</p>` : ''}
                        ${topicType ? `<span class="inline-block mt-1 px-2 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300 rounded">${topicType}</span>` : ''}
                    </div>
                    ${!isP2P ? `<div><h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Members</h4><div class="space-y-2">${this._renderMembers()}</div></div>` : ''}
                    <div>
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">About</h4>
                        <div class="text-sm text-gray-500 space-y-1">
                            <p>Topic: <span class="font-mono text-xs">${this.currentTopic}</span></p>
                            ${info?.seq ? `<p>Messages: ${info.seq}</p>` : ''}
                            ${info?.public?.slug ? `<p>Slug: ${info.public.slug}</p>` : ''}
                        </div>
                    </div>

                    ${isGroup ? `
                    <div class="pt-4 border-t border-gray-200 dark:border-gray-700">
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Actions</h4>
                        <button id="leave-topic-btn" class="w-full px-4 py-2 bg-yellow-600 hover:bg-yellow-700 text-white rounded-lg text-sm font-medium mb-2">
                            Leave ${topicType === 'workspace' ? 'Workspace' : 'Group'}
                        </button>
                        ${isOwner ? `
                        <button id="delete-topic-btn" class="w-full px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg text-sm font-medium">
                            Delete ${topicType === 'workspace' ? 'Workspace' : 'Group'}
                        </button>
                        <p class="text-xs text-gray-500 mt-2">Deleting removes this for all members.</p>
                        ` : ''}
                    </div>
                    ` : ''}
                </div>
            `;

            // Leave topic handler
            document.getElementById('leave-topic-btn')?.addEventListener('click', async () => {
                const name = info?.public?.fn || this.currentTopic;
                if (!confirm(`Leave "${name}"? You'll need to be invited back to rejoin.`)) return;
                try {
                    await this._leaveTopic(this.currentTopic);
                    this.rightPanel?.classList.add('hidden');
                    notifications.success(`Left "${name}"`);
                } catch (err) {
                    notifications.error(`Failed to leave: ${err.message}`);
                }
            });

            // Delete topic handler
            document.getElementById('delete-topic-btn')?.addEventListener('click', async () => {
                const name = info?.public?.fn || this.currentTopic;
                if (!confirm(`Delete "${name}"? This cannot be undone!`)) return;
                if (!confirm(`Are you sure? All messages will be permanently deleted.`)) return;
                try {
                    await this._deleteTopic(this.currentTopic);
                    this.rightPanel?.classList.add('hidden');
                    notifications.success(`"${name}" deleted`);
                } catch (err) {
                    notifications.error(`Failed to delete: ${err.message}`);
                }
            });
            }
            this.rightPanel?.classList.remove('hidden');
        } catch (err) {
            console.error('Error showing topic details:', err);
        }
    }

    async _leaveTopic(topicName) {
        const topic = this.tinode.client.getTopic(topicName);
        if (!topic.isSubscribed()) {
            await topic.subscribe();
        }
        await topic.leave(true); // true = delete subscription

        // Clear current topic and refresh
        this.currentTopic = null;
        this.messages.showWelcome();
        this.composer.disable();
        this.sidebar.selectTopic(null);
        this._updateSubscriptions();

        // If it was a workspace, remove from workspace list
        const workspaceIndex = this.sidebar.workspaces.findIndex(w => w.id === topicName);
        if (workspaceIndex >= 0) {
            this.sidebar.workspaces.splice(workspaceIndex, 1);
            if (this.sidebar.workspaces.length > 0) {
                this.sidebar.setCurrentWorkspace(this.sidebar.workspaces[0].id);
            } else {
                this.sidebar.currentWorkspace = null;
                this.sidebar._updateWorkspaceDisplay();
            }
        }
    }

    async _deleteTopic(topicName) {
        const topic = this.tinode.client.getTopic(topicName);
        if (!topic.isSubscribed()) {
            await topic.subscribe();
        }
        // Tinode SDK uses delTopic() to delete a topic
        await topic.delTopic(true); // true = hard delete

        // Clear current topic and refresh
        this.currentTopic = null;
        this.messages.showWelcome();
        this.composer.disable();
        this.sidebar.selectTopic(null);
        this._updateSubscriptions();

        // If it was a workspace, remove from workspace list
        const workspaceIndex = this.sidebar.workspaces.findIndex(w => w.id === topicName);
        if (workspaceIndex >= 0) {
            this.sidebar.workspaces.splice(workspaceIndex, 1);
            if (this.sidebar.workspaces.length > 0) {
                this.sidebar.setCurrentWorkspace(this.sidebar.workspaces[0].id);
            } else {
                this.sidebar.currentWorkspace = null;
                this.sidebar._updateWorkspaceDisplay();
            }
        }
    }

    _showMembersPanel() {
        if (this.panelTitle) this.panelTitle.textContent = 'Members';
        if (this.panelContent) {
            const isGroup = this.currentTopic?.startsWith('grp');
            const canInvite = this.tinode.canPerform?.('invite') || false;
            const myPerms = this.tinode.getMyPermissions?.() || null;

            this.panelContent.innerHTML = `
                ${myPerms ? `
                <div class="mb-3 px-2 py-1.5 bg-gray-50 dark:bg-gray-700 rounded text-xs text-gray-600 dark:text-gray-300">
                    Your role: <span class="font-medium">${myPerms.role}</span>
                    ${myPerms.canInvite ? ' · Can invite' : ''}
                    ${myPerms.canRemoveMembers ? ' · Can remove' : ''}
                </div>
                ` : ''}
                ${isGroup && canInvite ? `
                <div class="mb-4">
                    <button id="add-member-btn" class="w-full px-3 py-2 bg-slack-accent text-white rounded hover:bg-opacity-90 text-sm font-medium flex items-center justify-center space-x-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
                        </svg>
                        <span>Add Member</span>
                    </button>
                </div>
                ` : isGroup ? `
                <div class="mb-4 px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded text-sm text-gray-500 dark:text-gray-400 text-center">
                    Only admins can add members
                </div>
                ` : ''}
                <div id="members-list" class="space-y-2">${this._renderMembers()}</div>
            `;

            // Add event listener for add member button
            document.getElementById('add-member-btn')?.addEventListener('click', () => {
                this._showAddMemberModal();
            });

            // Add event listeners for remove buttons
            this.panelContent.querySelectorAll('[data-remove-member]').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    const userId = e.currentTarget.dataset.removeMember;
                    this._removeMember(userId);
                });
            });
        }
        this.rightPanel?.classList.remove('hidden');
    }

    _renderMembers() {
        try {
            const members = this.tinode.getTopicMembers() || [];
            if (members.length === 0) return '<p class="text-sm text-gray-500">No members</p>';

            const isGroup = this.currentTopic?.startsWith('grp');
            const canRemove = this.tinode.canPerform?.('removeMember') || false;

            // Phase 2: Merge agent status from workspace agents
            const workspaceAgents = this.sidebar?.workspaceAgents || [];
            members.forEach(m => {
                // Check if this member is a workspace agent by tinodeUserId
                const agent = workspaceAgents.find(a => a.tinodeUserId === m.id);
                if (agent) {
                    m.isBot = true;
                    // Agent is online if status is 'running' or 'dev'
                    m.online = agent.status === 'running' || agent.status === 'dev';
                }
            });

        return members.map(m => {
            // Get member's role/permissions
            const memberPerms = this.tinode.getMemberPermissions(m.id);
            const role = memberPerms?.role || 'Member';
            const isMe = m.id === this.tinode.getUserId();

            // Role badge styling
            const roleBadge = role === 'Owner'
                ? '<span class="ml-1 px-1.5 py-0.5 text-xs bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300 rounded">Owner</span>'
                : role === 'Admin'
                ? '<span class="ml-1 px-1.5 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300 rounded">Admin</span>'
                : role === 'Moderator'
                ? '<span class="ml-1 px-1.5 py-0.5 text-xs bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300 rounded">Mod</span>'
                : '';

            return `
            <div class="flex items-center space-x-3 p-2 rounded hover:bg-gray-100 dark:hover:bg-gray-700 group">
                <div class="relative">
                    ${m.isBot ? `
                    <div class="w-8 h-8 rounded bg-purple-500 flex items-center justify-center text-white text-sm">
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                        </svg>
                    </div>
                    ` : `
                    <div class="w-8 h-8 rounded bg-gray-400 flex items-center justify-center text-white text-sm">
                        ${(m.name || m.id).charAt(0).toUpperCase()}
                    </div>
                    `}
                    ${m.online ? '<span class="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 bg-slack-online border-2 border-white dark:border-gray-800 rounded-full"></span>' : ''}
                </div>
                <div class="flex-1 min-w-0">
                    <p class="text-sm font-medium text-gray-900 dark:text-white truncate flex items-center">
                        ${m.name || m.id}${isMe ? ' (you)' : ''}
                        ${m.isBot ? '<span class="ml-1 px-1.5 py-0.5 text-xs bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300 rounded">BOT</span>' : ''}
                        ${roleBadge}
                    </p>
                    <p class="text-xs text-gray-500">${m.online ? 'Online' : 'Offline'}</p>
                </div>
                ${isGroup && !isMe && canRemove ? `
                <button data-remove-member="${m.id}" class="hidden group-hover:block p-1 text-gray-400 hover:text-red-500" title="Remove from group">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                    </svg>
                </button>
                ` : ''}
            </div>
        `}).join('');
        } catch (err) {
            console.error('Error rendering members:', err);
            return '<p class="text-sm text-red-500">Error loading members</p>';
        }
    }

    _showAddMemberModal() {
        // Use the existing search component to find and add users
        this.search.open();
        // Override the select handler temporarily
        const originalOnSelect = this.search.onSelect;
        this.search.onSelect = async (userId, data) => {
            try {
                await this.tinode.inviteMember(userId);
                notifications.success(`Added ${data.name} to group`);
                this._showMembersPanel(); // Refresh members list
            } catch (err) {
                notifications.error(`Failed to add member: ${err.message}`);
            }
            this.search.onSelect = originalOnSelect;
        };
    }

    async _removeMember(userId) {
        const member = this.tinode.getTopicMembers().find(m => m.id === userId);
        const name = member?.name || userId;

        if (!confirm(`Remove ${name} from this group?`)) return;

        try {
            await this.tinode.removeMember(userId);
            notifications.success(`Removed ${name} from group`);
            this._showMembersPanel(); // Refresh
        } catch (err) {
            notifications.error(`Failed to remove member: ${err.message}`);
        }
    }

    _showLoginError(message) {
        if (this.loginError) {
            this.loginError.textContent = message;
            this.loginError.classList.remove('hidden');
        }
    }

    _hideLoginError() {
        this.loginError?.classList.add('hidden');
    }

    // ==========================================
    // Workspace Settings
    // ==========================================

    _showWorkspaceSettings(workspace) {
        if (!workspace) {
            notifications.error('No workspace selected');
            return;
        }

        if (this.panelTitle) this.panelTitle.textContent = 'Workspace Settings';
        if (this.panelContent) {
            this.panelContent.innerHTML = `
                <div class="space-y-6">
                    <div class="text-center">
                        <div class="w-20 h-20 mx-auto rounded-lg bg-slack-accent flex items-center justify-center text-white text-3xl font-medium">
                            ${(workspace.name || workspace.slug || 'W').charAt(0).toUpperCase()}
                        </div>
                        <h3 class="mt-4 text-lg font-bold text-gray-900 dark:text-white">
                            ${workspace.name || workspace.slug}
                        </h3>
                        <p class="text-sm text-gray-500">${workspace.slug || workspace.id}</p>
                    </div>

                    <div>
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Details</h4>
                        <div class="text-sm text-gray-500 space-y-1">
                            <p><span class="text-gray-400">Topic ID:</span> <span class="font-mono text-xs">${workspace.id || workspace.topic}</span></p>
                            <p><span class="text-gray-400">Owner:</span> ${workspace.owner === this.tinode?.getUserId() ? 'You' : workspace.owner || 'Unknown'}</p>
                            ${workspace.description ? `<p><span class="text-gray-400">Description:</span> ${workspace.description}</p>` : ''}
                        </div>
                    </div>

                    <div class="pt-4 border-t border-gray-200 dark:border-gray-700">
                        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Danger Zone</h4>
                        <button id="leave-workspace-btn" class="w-full px-4 py-2 bg-yellow-600 hover:bg-yellow-700 text-white rounded-lg text-sm font-medium mb-2">
                            Leave Workspace
                        </button>
                        ${workspace.owner === this.tinode?.getUserId() ? `
                        <button id="delete-workspace-btn" class="w-full px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg text-sm font-medium">
                            Delete Workspace
                        </button>
                        <p class="text-xs text-gray-500 mt-2">Deleting will remove this workspace for all members.</p>
                        ` : ''}
                    </div>
                </div>
            `;

            // Leave workspace handler
            document.getElementById('leave-workspace-btn')?.addEventListener('click', async () => {
                if (!confirm(`Leave workspace "${workspace.name}"? You'll need to be invited back to rejoin.`)) return;
                try {
                    await this._leaveWorkspace(workspace);
                    this.rightPanel?.classList.add('hidden');
                    notifications.success(`Left workspace "${workspace.name}"`);
                } catch (err) {
                    notifications.error(`Failed to leave: ${err.message}`);
                }
            });

            // Delete workspace handler
            document.getElementById('delete-workspace-btn')?.addEventListener('click', async () => {
                if (!confirm(`Delete workspace "${workspace.name}"? This cannot be undone!`)) return;
                if (!confirm(`Are you sure? All channels and messages will be permanently deleted.`)) return;
                try {
                    await this._deleteWorkspace(workspace);
                    this.rightPanel?.classList.add('hidden');
                    notifications.success(`Workspace "${workspace.name}" deleted`);
                } catch (err) {
                    notifications.error(`Failed to delete: ${err.message}`);
                }
            });
        }

        this.rightPanel?.classList.remove('hidden');
    }

    async _leaveWorkspace(workspace) {
        const topic = this.tinode.client.getTopic(workspace.id);
        if (!topic.isSubscribed()) {
            await topic.subscribe();
        }
        await topic.leave(true); // true = delete subscription

        // Remove from local list and switch to another workspace
        this.sidebar.workspaces = this.sidebar.workspaces.filter(w => w.id !== workspace.id);
        if (this.sidebar.workspaces.length > 0) {
            this.sidebar.setCurrentWorkspace(this.sidebar.workspaces[0].id);
        } else {
            this.sidebar.currentWorkspace = null;
            this.sidebar._updateWorkspaceDisplay();
        }
        this._updateSubscriptions();
    }

    async _deleteWorkspace(workspace) {
        const topic = this.tinode.client.getTopic(workspace.id);
        if (!topic.isSubscribed()) {
            await topic.subscribe();
        }
        await topic.delTopic(true); // true = hard delete

        // Remove from local list
        this.sidebar.workspaces = this.sidebar.workspaces.filter(w => w.id !== workspace.id);
        if (this.sidebar.workspaces.length > 0) {
            this.sidebar.setCurrentWorkspace(this.sidebar.workspaces[0].id);
        } else {
            this.sidebar.currentWorkspace = null;
            this.sidebar._updateWorkspaceDisplay();
        }
        this._updateSubscriptions();
    }

    // ==========================================
    // SDK Settings
    // ==========================================

    async _showSDKSettings(workspace) {
        if (!workspace) {
            notifications.error('No workspace selected');
            return;
        }

        // Update modal content
        document.getElementById('sdk-workspace-name').textContent = workspace.name || workspace.slug || '-';
        document.getElementById('sdk-workspace-id').textContent = workspace.id || workspace.topic || '-';
        document.getElementById('sdk-user-email').textContent = window.authManager?.user?.email || '-';
        document.getElementById('sdk-pocketnode-url').textContent = this._getPocketNodeUrl();

        // Show modal
        this.sdkSettingsModal?.classList.remove('hidden');

        // Setup event handlers
        this._setupSDKSettingsHandlers();
    }

    _setupSDKSettingsHandlers() {
        // Close button
        document.getElementById('close-sdk-settings-btn')?.addEventListener('click', () => {
            this.sdkSettingsModal?.classList.add('hidden');
        });

        // Click outside to close
        this.sdkSettingsModal?.addEventListener('click', (e) => {
            if (e.target === this.sdkSettingsModal) {
                this.sdkSettingsModal.classList.add('hidden');
            }
        });

        // Download config
        document.getElementById('download-config-btn')?.addEventListener('click', () => {
            this._downloadSDKConfig();
        });
    }

    /**
     * Get the PocketNode URL (for SDK config)
     * In production, nginx proxies to PocketNode
     * In dev, connect directly on port 8090
     */
    _getPocketNodeUrl() {
        const hostname = window.location.hostname;
        const isProduction = hostname !== 'localhost' && hostname !== '127.0.0.1';
        const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:';

        if (isProduction) {
            // In production, use same origin (nginx proxies to PocketNode)
            return `${protocol}//${window.location.host}`;
        }
        // In development, connect directly to PocketNode on port 8090
        return `http://${hostname}:8090`;
    }

    _downloadSDKConfig() {
        const workspace = this.currentWorkspace;
        const user = window.authManager?.user;
        const pb = window.authManager?.pb;

        if (!workspace || !user) {
            notifications.error('No workspace selected');
            return;
        }

        if (!pb?.authStore?.token) {
            notifications.error('Not authenticated. Please sign in again.');
            return;
        }

        // Get channels in this workspace
        const workspaceId = workspace.id || workspace.topic;
        const channels = this._getWorkspaceChannels(workspaceId);

        // New SDK config format using PocketBase auth
        const config = {
            pocketnode_url: this._getPocketNodeUrl(),
            workspace: workspaceId,
            channels: channels,  // Include channel IDs for agent subscription
            auth_token: pb.authStore.token,
            // Metadata (for reference, not used by SDK)
            _metadata: {
                workspace_name: workspace.name || workspace.slug,
                user_email: user.email,
                created_at: new Date().toISOString(),
                note: "This auth token expires. Re-download from the web UI if connection fails."
            }
        };

        // Create and download file
        const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'gather.config.json';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        notifications.success('Config downloaded');
    }

    /**
     * Get channel IDs for a workspace from current subscriptions
     */
    _getWorkspaceChannels(workspaceId) {
        const channels = [];
        const { groups } = this.tinode?.getSubscriptions() || {};

        for (const group of (groups || [])) {
            // Check if this is a channel belonging to the workspace
            const publicData = group.public || group.publicData;
            if (publicData?.type === 'channel' && publicData?.parent === workspaceId) {
                channels.push(group.topic);
            }
        }

        console.log('[App] Found channels for workspace:', channels);
        return channels;
    }

    // ==========================================
    // Create Agent Modal
    // ==========================================

    async _showCreateAgentModal() {
        if (!this.currentWorkspace) {
            notifications.error('No workspace selected');
            return;
        }

        // Load tools library
        await this._loadToolsLibrary();

        // Reset form
        this.createAgentForm?.reset();
        document.getElementById('create-agent-error')?.classList.add('hidden');

        // Reset agent type selection to managed
        document.querySelector('input[name="agent-type"][value="managed"]').checked = true;
        document.getElementById('managed-agent-fields')?.classList.remove('hidden');
        document.getElementById('webhook-agent-fields')?.classList.add('hidden');

        // Render tools list
        this._renderToolsList();

        // Show modal
        this.createAgentModal?.classList.remove('hidden');

        // Setup event handlers
        this._setupCreateAgentHandlers();
    }

    async _loadToolsLibrary() {
        if (this.toolsLibrary.length === 0) {
            this.toolsLibrary = await window.authManager?.getToolsLibrary() || [];
        }
    }

    _renderToolsList() {
        const container = document.getElementById('tools-list');
        if (!container) return;

        if (this.toolsLibrary.length === 0) {
            container.innerHTML = '<div class="text-sm text-gray-500 dark:text-gray-400">No tools available</div>';
            return;
        }

        container.innerHTML = this.toolsLibrary.map(tool => `
            <label class="flex items-start space-x-2 cursor-pointer">
                <input type="checkbox" name="agent-tools" value="${tool.name}"
                    class="mt-1 rounded border-gray-300 dark:border-gray-600 text-slack-accent focus:ring-slack-accent">
                <div class="flex-1">
                    <div class="text-sm font-medium text-gray-900 dark:text-white">${tool.displayName}</div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">${tool.description}</div>
                </div>
            </label>
        `).join('');
    }

    _setupCreateAgentHandlers() {
        // Only add listeners once
        if (this._createAgentHandlersSetup) return;
        this._createAgentHandlersSetup = true;

        // Close buttons
        document.getElementById('close-create-agent-btn')?.addEventListener('click', () => {
            this.createAgentModal?.classList.add('hidden');
        });

        document.getElementById('cancel-create-agent-btn')?.addEventListener('click', () => {
            this.createAgentModal?.classList.add('hidden');
        });

        // Click outside to close
        this.createAgentModal?.addEventListener('click', (e) => {
            if (e.target === this.createAgentModal) {
                this.createAgentModal.classList.add('hidden');
            }
        });

        // Agent type toggle
        document.querySelectorAll('input[name="agent-type"]').forEach(radio => {
            radio.addEventListener('change', (e) => {
                const isManaged = e.target.value === 'managed';
                document.getElementById('managed-agent-fields')?.classList.toggle('hidden', !isManaged);
                document.getElementById('webhook-agent-fields')?.classList.toggle('hidden', isManaged);
            });
        });

        // Form submission
        this.createAgentForm?.addEventListener('submit', async (e) => {
            e.preventDefault();
            await this._handleCreateAgent();
        });
    }

    // ==========================================
    // Mobile Navigation
    // ==========================================

    _toggleSidebar() {
        if (this.sidebarOpen) {
            this._closeSidebar();
        } else {
            this._openSidebar();
        }
    }

    _openSidebar() {
        const sidebar = document.getElementById('sidebar');
        const backdrop = document.getElementById('sidebar-backdrop');

        sidebar?.classList.add('sidebar-open');
        backdrop?.classList.add('active');
        this.sidebarOpen = true;
    }

    _closeSidebar() {
        const sidebar = document.getElementById('sidebar');
        const backdrop = document.getElementById('sidebar-backdrop');

        sidebar?.classList.remove('sidebar-open');
        backdrop?.classList.remove('active');
        this.sidebarOpen = false;
    }

    _setupSwipeGestures() {
        let touchStartX = 0;
        let touchStartY = 0;
        let touchCurrentX = 0;
        let isSwiping = false;
        const swipeThreshold = 50;
        const edgeThreshold = 30; // Start swipe from edge

        const appContainer = document.getElementById('app-container');
        const sidebar = document.getElementById('sidebar');

        if (!appContainer) return;

        appContainer.addEventListener('touchstart', (e) => {
            if (!this.isMobile) return;

            touchStartX = e.touches[0].clientX;
            touchStartY = e.touches[0].clientY;
            touchCurrentX = touchStartX;

            // Only start swipe if touching near left edge (to open) or sidebar is open
            isSwiping = touchStartX < edgeThreshold || this.sidebarOpen;
        }, { passive: true });

        appContainer.addEventListener('touchmove', (e) => {
            if (!this.isMobile || !isSwiping) return;

            touchCurrentX = e.touches[0].clientX;
            const diffY = Math.abs(e.touches[0].clientY - touchStartY);
            const diffX = touchCurrentX - touchStartX;

            // Cancel if vertical scroll is dominant
            if (diffY > Math.abs(diffX)) {
                isSwiping = false;
                return;
            }

            // Visual feedback during swipe
            if (sidebar && !this.sidebarOpen && diffX > 0) {
                const progress = Math.min(diffX / 250, 1);
                sidebar.style.transform = `translateX(${-100 + progress * 100}%)`;
            } else if (sidebar && this.sidebarOpen && diffX < 0) {
                const progress = Math.max(diffX / 250, -1);
                sidebar.style.transform = `translateX(${progress * 100}%)`;
            }
        }, { passive: true });

        appContainer.addEventListener('touchend', (e) => {
            if (!this.isMobile || !isSwiping) return;

            const diffX = touchCurrentX - touchStartX;

            // Reset inline transform
            if (sidebar) {
                sidebar.style.transform = '';
            }

            // Open sidebar with right swipe from edge
            if (!this.sidebarOpen && diffX > swipeThreshold && touchStartX < edgeThreshold) {
                this._openSidebar();
            }
            // Close sidebar with left swipe
            else if (this.sidebarOpen && diffX < -swipeThreshold) {
                this._closeSidebar();
            }

            isSwiping = false;
        }, { passive: true });
    }

    async _handleCreateAgent() {
        const errorEl = document.getElementById('create-agent-error');
        const submitBtn = document.getElementById('submit-create-agent-btn');

        try {
            // Disable button while submitting
            if (submitBtn) {
                submitBtn.disabled = true;
                submitBtn.textContent = 'Creating...';
            }
            errorEl?.classList.add('hidden');

            // Get form values
            const agentType = document.querySelector('input[name="agent-type"]:checked')?.value || 'managed';
            const handle = document.getElementById('agent-handle-input')?.value?.trim().toLowerCase();
            const displayName = document.getElementById('agent-name-input')?.value?.trim();
            const description = document.getElementById('agent-description-input')?.value?.trim();

            // Validate required fields
            if (!handle) throw new Error('Handle is required');
            if (!displayName) throw new Error('Display name is required');
            if (!/^[a-z0-9_]+$/.test(handle)) throw new Error('Handle must be lowercase letters, numbers, and underscores only');

            let agentData = {
                handle,
                displayName,
                description,
                agentType,
            };

            if (agentType === 'managed') {
                agentData.prompt = document.getElementById('agent-prompt-input')?.value?.trim() || '';
                agentData.model = document.getElementById('agent-model-select')?.value || 'gemini-2.5-flash';

                // Get selected tools
                const selectedTools = Array.from(document.querySelectorAll('input[name="agent-tools"]:checked'))
                    .map(cb => cb.value);
                agentData.tools = selectedTools;
            } else {
                const endpointUrl = document.getElementById('agent-endpoint-input')?.value?.trim();
                if (!endpointUrl) throw new Error('Endpoint URL is required for webhook agents');
                agentData.endpointUrl = endpointUrl;
                agentData.timeoutMs = parseInt(document.getElementById('agent-timeout-input')?.value) || 30000;
            }

            // Create the agent
            const agent = await window.authManager.createAgent(this.currentWorkspace.id, agentData);

            // Close modal and refresh agents list
            this.createAgentModal?.classList.add('hidden');
            notifications.success(`Agent @${agent.handle} created!`);

            // Refresh agents list
            await this._loadWorkspaceAgents(this.currentWorkspace.id);

        } catch (err) {
            console.error('[App] Failed to create agent:', err);
            if (errorEl) {
                errorEl.textContent = err.message;
                errorEl.classList.remove('hidden');
            }
        } finally {
            if (submitBtn) {
                submitBtn.disabled = false;
                submitBtn.textContent = 'Create Agent';
            }
        }
    }
}

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    window.app = new App();
    window.app.init();
});
