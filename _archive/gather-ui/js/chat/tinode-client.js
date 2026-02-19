/**
 * Tinode SDK Wrapper
 * Clean, minimal wrapper around Tinode SDK
 */

class TinodeClient {
    constructor(config = {}) {
        this.config = {
            host: config.host || 'localhost:6060',
            secure: config.secure || false,
            apiKey: config.apiKey || 'AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K',
            appName: config.appName || 'TinodeSlackChat',
            transport: config.transport || 'ws'
        };

        this.client = null;
        this.currentTopic = null;
        this.meTopic = null;
        this.fndTopic = null;
        this.myUserId = null;
        this.isConnected = false;
        this.isLoggedIn = false;

        // User name cache - persists across topic switches
        // Populated from topic subscriptions and messages
        this._userCache = this._loadUserCache();

        // Event callbacks
        this.onConnectionChange = null;
        this.onLoginSuccess = null;
        this.onLoginFailed = null;
        this.onMessage = null;
        this.onPresence = null;
        this.onTyping = null;
        this.onTopicUpdate = null;
        this.onSearchResults = null;
        this.onTopicLeave = null;
    }

    init() {
        const TinodeSDK = window.tinode;
        if (!TinodeSDK) throw new Error('Tinode SDK not loaded');

        this.client = new TinodeSDK.Tinode({
            appName: this.config.appName,
            host: this.config.host,
            apiKey: this.config.apiKey,
            secure: this.config.secure,
            transport: this.config.transport
        });

        this.client.onConnect = () => {
            this.isConnected = true;
            this._emit('onConnectionChange', { connected: true });
        };

        this.client.onDisconnect = (err) => {
            this.isConnected = false;
            this.isLoggedIn = false;
            this._emit('onConnectionChange', { connected: false, error: err });
        };

        this.client.onPresMessage = (pres) => {
            this._emit('onPresence', pres);
        };

        return this;
    }

    _emit(event, data) {
        if (typeof this[event] === 'function') this[event](data);
    }

    async connect() {
        if (!this.client) throw new Error('Client not initialized');
        return this.client.connect();
    }

    async login(username, password) {
        try {
            const ctrl = await this.client.loginBasic(username, password);
            this.myUserId = this.client.getCurrentUserID();
            this.isLoggedIn = true;
            this._emit('onLoginSuccess', { userId: this.myUserId, ctrl });
            return ctrl;
        } catch (err) {
            this._emit('onLoginFailed', err);
            throw err;
        }
    }

    /**
     * Login using REST auth scheme with PocketBase token
     * @param {string} pbToken - PocketBase auth token
     */
    async loginWithRestAuth(pbToken) {
        try {
            // Encode the PocketBase token as base64 secret
            const secret = btoa(pbToken);

            // Use loginToken with "rest" scheme - Tinode will call our REST auth endpoint
            const ctrl = await this.client.login('rest', secret);

            this.myUserId = this.client.getCurrentUserID();
            this.isLoggedIn = true;
            this._emit('onLoginSuccess', { userId: this.myUserId, ctrl });
            return ctrl;
        } catch (err) {
            console.error('[TinodeClient] REST auth failed:', err);
            this._emit('onLoginFailed', err);
            throw err;
        }
    }

    getUserId() {
        return this.myUserId;
    }

    async subscribeToMe() {
        this.meTopic = this.client.getMeTopic();

        // Promise that resolves when subscriptions are loaded
        const subsLoaded = new Promise((resolve) => {
            this.meTopic.onSubsUpdated = () => {
                console.log('[TinodeClient] Subscriptions loaded');
                this._emit('onTopicUpdate', this.getSubscriptions());
                resolve();
            };
        });

        this.meTopic.onMetaSub = (sub) => {
            // Cache the topic's public data when we receive subscription updates
            if (sub && sub.topic && sub.public) {
                const topic = this.client.getTopic(sub.topic);
                if (topic && !topic.public) {
                    topic.public = sub.public;
                }
                // For P2P topics (usr...), cache the user's name
                if (sub.topic.startsWith('usr') && sub.public.fn) {
                    this.cacheUserInfo(sub.topic, sub.public.fn, !!sub.public.bot);
                }
            }
            this._emit('onSubscriptionUpdate', sub);
        };

        this.meTopic.onContactUpdate = () => {
            this._emit('onTopicUpdate', this.getSubscriptions());
        };

        const getQuery = this.meTopic.startMetaQuery()
            .withSub()
            .withDesc()
            .build();

        await this.meTopic.subscribe(getQuery);

        // Wait for subscriptions to be processed (with timeout)
        await Promise.race([
            subsLoaded,
            new Promise(r => setTimeout(r, 2000)) // 2s timeout
        ]);

        return this.meTopic;
    }

    getSubscriptions() {
        if (!this.meTopic) return { groups: [], dms: [] };

        const groups = [];
        const dms = [];

        this.meTopic.contacts((sub) => {
            // Skip invalid subscriptions
            if (!sub || !sub.topic) return;

            // Try multiple sources for the name and metadata
            let publicData = sub.public;
            if (!publicData) {
                const topic = this.client.getTopic(sub.topic);
                publicData = topic?.public;
            }

            const name = publicData?.fn || sub.topic;
            const photo = publicData?.photo?.data;
            const workspaceId = publicData?.workspace_id || publicData?.parent;
            const topicType = publicData?.type; // 'workspace', 'channel', or undefined

            const item = {
                topic: sub.topic,
                name: name,
                photo: this._photoToDataUri(photo),
                updated: sub.updated,
                touched: sub.touched,
                read: sub.read,
                recv: sub.recv,
                unread: sub.seq - (sub.read || 0),
                online: sub.online,
                isP2P: sub.topic?.startsWith('usr'),
                isGroup: sub.topic?.startsWith('grp'),
                isBot: this._isBotName(name),
                workspaceId: workspaceId || null,
                type: topicType
            };

            // Only add items with valid topic
            if (item.topic) {
                if (item.isP2P) {
                    dms.push(item);
                } else if (item.isGroup) {
                    // Skip workspaces - they show in the workspace dropdown, not groups list
                    if (topicType !== 'workspace') {
                        groups.push(item);
                    }
                }
            }
        });

        const sortByTouch = (a, b) => new Date(b.touched || b.updated) - new Date(a.touched || a.updated);
        groups.sort(sortByTouch);
        dms.sort(sortByTouch);

        return { groups, dms };
    }

    async subscribeTopic(topicName) {
        // Save messages from current topic before leaving (for persistence)
        if (this.currentTopic?.isSubscribed()) {
            const currentTopicName = this.currentTopic.name;
            const messages = this.getCachedMessages();
            if (messages.length > 0 && currentTopicName) {
                this._emit('onTopicLeave', { topic: currentTopicName, messages });
            }
            await this.currentTopic.leave(false);
        }

        const topic = this.client.getTopic(topicName);
        this.currentTopic = topic;

        topic.onData = (data) => {
            // Skip incomplete local messages (SDK fires onData before server confirms)
            if (!data.from) return;

            this._emit('onMessage', {
                seq: data.seq,
                from: data.from,
                content: this._extractContent(data.content),
                rawContent: data.content,
                ts: data.ts,
                isOwn: data.from === this.myUserId,
                topic: topicName
            });
        };

        topic.onMeta = (meta) => {
            if (meta.desc) {
                this._emit('onTopicUpdate', { topic: topicName, desc: meta.desc });
            }
        };

        topic.onPres = (pres) => {
            if (pres.what === 'kp') {
                this._emit('onTyping', { topic: topicName, from: pres.src });
            }
            this._emit('onPresence', pres);
        };

        const getQuery = topic.startMetaQuery()
            .withLaterData(50)
            .withLaterSub()
            .withDesc()
            .build();

        return topic.subscribe(getQuery);
    }

    _extractContent(content) {
        if (!content) return '';
        if (typeof content === 'string') return content;
        if (content.txt) return content.txt;
        if (content.text) return content.text;
        if (content.content) {
            return typeof content.content === 'string' ? content.content : this._extractContent(content.content);
        }
        if (content.message) return content.message;
        if (content.body) return content.body;
        if (content.data?.txt) return content.data.txt;
        return JSON.stringify(content);
    }

    /**
     * Send message - noEcho=false so server echoes it back through onData
     */
    async sendMessage(text) {
        if (!this.currentTopic?.isSubscribed()) {
            throw new Error('Not subscribed to any topic');
        }
        // noEcho: false - server will echo back, message appears via onData
        return this.currentTopic.publish({ txt: text }, false);
    }

    noteKeyPress() {
        if (this.currentTopic?.isSubscribed()) {
            this.currentTopic.noteKeyPress();
        }
    }

    noteRead(seq) {
        if (this.currentTopic?.isSubscribed()) {
            this.currentTopic.noteRead(seq);
        }
    }

    getTopicInfo(topicName) {
        const topic = this.client.getTopic(topicName);
        if (!topic) return null;

        return {
            name: topicName,
            public: topic.public,
            private: topic.private,
            isSubscribed: topic.isSubscribed(),
            seq: topic.seq,
            read: topic.read,
            recv: topic.recv
        };
    }

    /**
     * Tinode permission flags:
     * J = Join (can subscribe to topic)
     * R = Read (can read messages)
     * W = Write (can publish messages)
     * P = Presence (can see online status)
     * A = Admin (can change topic settings, remove members)
     * S = Share (can invite others)
     * D = Delete (can hard-delete messages)
     * O = Owner (full control, can delete topic)
     */

    /**
     * Get current user's permissions on the current topic
     * Returns object with boolean flags for each permission
     */
    getMyPermissions() {
        if (!this.currentTopic) return null;

        // Get the access mode - SDK provides this via getAccessMode() or .acs property
        let mode = '';
        try {
            const acs = this.currentTopic.getAccessMode?.() || this.currentTopic.acs;
            // acs.mode might be a string or an AccessMode object with getMode() method
            if (acs) {
                if (typeof acs === 'string') {
                    mode = acs;
                } else if (typeof acs.getMode === 'function') {
                    mode = acs.getMode();
                } else if (typeof acs.mode === 'string') {
                    mode = acs.mode;
                } else if (acs.mode && typeof acs.mode.getMode === 'function') {
                    mode = acs.mode.getMode();
                } else if (acs.toString) {
                    mode = acs.toString();
                }
            }
            // Ensure mode is a string
            if (typeof mode !== 'string') {
                mode = String(mode || '');
            }
        } catch (e) {
            console.warn('Could not get access mode:', e);
            return null;
        }
        if (!mode || mode === 'undefined' || mode === 'null') return null;

        return {
            // Raw mode string
            mode: mode,
            // Individual permissions
            canJoin: mode.includes('J'),
            canRead: mode.includes('R'),
            canWrite: mode.includes('W'),
            canPresence: mode.includes('P'),
            canAdmin: mode.includes('A'),
            canShare: mode.includes('S'),
            canDelete: mode.includes('D'),
            isOwner: mode.includes('O'),
            // Computed helpers
            canInvite: mode.includes('S') || mode.includes('A') || mode.includes('O'),
            canRemoveMembers: mode.includes('A') || mode.includes('O'),
            canEditTopic: mode.includes('A') || mode.includes('O'),
            canDeleteTopic: mode.includes('O'),
            // Role name for display
            role: mode.includes('O') ? 'Owner'
                : mode.includes('A') ? 'Admin'
                : mode.includes('S') ? 'Moderator'
                : 'Member'
        };
    }

    /**
     * Get permissions for a specific user in the current topic
     */
    getMemberPermissions(userId) {
        try {
            if (!this.currentTopic) return null;

            // Try to get subscriber info
            let sub = null;
            try {
                sub = this.currentTopic.subscriber?.(userId);
            } catch (e) {
                // subscriber() might not exist, try iterating
            }

            if (!sub) {
                // Fallback: search through subscribers
                this.currentTopic.subscribers?.((s) => {
                    if (s.user === userId) sub = s;
                });
            }

            if (!sub) return null;

            const acs = sub.acs;
            let mode = '';
            if (acs) {
                if (typeof acs === 'string') {
                    mode = acs;
                } else if (typeof acs.getMode === 'function') {
                    mode = acs.getMode();
                } else if (typeof acs.mode === 'string') {
                    mode = acs.mode;
                } else if (acs.mode && typeof acs.mode.getMode === 'function') {
                    mode = acs.mode.getMode();
                } else if (acs.toString) {
                    mode = acs.toString();
                }
            }
            if (typeof mode !== 'string') {
                mode = String(mode || '');
            }

            return {
                mode: mode,
                canAdmin: mode.includes('A'),
                canShare: mode.includes('S'),
                isOwner: mode.includes('O'),
                role: mode.includes('O') ? 'Owner'
                    : mode.includes('A') ? 'Admin'
                    : mode.includes('S') ? 'Moderator'
                    : 'Member'
            };
        } catch (err) {
            console.warn('Error getting member permissions:', err);
            return null;
        }
    }

    /**
     * Check if current user can perform a specific action
     */
    canPerform(action) {
        try {
            const perms = this.getMyPermissions();
            if (!perms) return false;

            switch (action) {
                case 'invite':
                case 'addMember':
                    return perms.canInvite;
                case 'removeMember':
                case 'kick':
                    return perms.canRemoveMembers;
                case 'editTopic':
                case 'changeName':
                case 'changeDescription':
                    return perms.canEditTopic;
                case 'deleteTopic':
                    return perms.canDeleteTopic;
                case 'deleteMessage':
                    return perms.canDelete || perms.canAdmin;
                case 'write':
                case 'send':
                    return perms.canWrite;
                default:
                    return false;
            }
        } catch (err) {
            console.warn('Error checking permissions:', err);
            return false;
        }
    }

    getCachedMessages() {
        if (!this.currentTopic) return [];

        const messages = [];
        this.currentTopic.messages((msg) => {
            messages.push({
                seq: msg.seq,
                from: msg.from,
                content: this._extractContent(msg.content),
                rawContent: msg.content,
                ts: msg.ts,
                isOwn: msg.from === this.myUserId
            });
        });
        return messages;
    }

    /**
     * Load user cache from localStorage
     */
    _loadUserCache() {
        try {
            const cached = localStorage.getItem('tinode_user_cache');
            return cached ? JSON.parse(cached) : {};
        } catch (e) {
            return {};
        }
    }

    /**
     * Save user cache to localStorage
     */
    _saveUserCache() {
        try {
            localStorage.setItem('tinode_user_cache', JSON.stringify(this._userCache));
        } catch (e) {
            // Ignore storage errors
        }
    }

    /**
     * Cache user info for later retrieval
     */
    cacheUserInfo(userId, name, isBot = false) {
        if (!userId || !name || name === userId) return;
        this._userCache[userId] = { name, isBot, ts: Date.now() };
        this._saveUserCache();
    }

    /**
     * Get user info - uses SDK's userDesc(), local cache, and contacts
     */
    getUserInfo(userId) {
        // Own user - get from meTopic
        if (userId === this.myUserId) {
            return {
                id: userId,
                name: this.meTopic?.public?.fn || 'Me',
                online: true,
                isBot: false
            };
        }

        // Try SDK's userDesc (global cache)
        if (this.currentTopic) {
            const user = this.currentTopic.userDesc(userId);
            if (user?.public?.fn) {
                const name = user.public.fn;
                const isBot = this.isBot(userId);
                // Update our cache
                this.cacheUserInfo(userId, name, isBot);
                return {
                    id: userId,
                    name: name,
                    online: user.online,
                    isBot: isBot
                };
            }
        }

        // P2P topic - other person's info is in topic.public
        if (this.currentTopic?.name?.startsWith('usr') && userId === this.currentTopic.name) {
            const name = this.currentTopic.public?.fn || userId;
            if (name !== userId) {
                const isBot = this._isBotName(name);
                this.cacheUserInfo(userId, name, isBot);
                return {
                    id: userId,
                    name: name,
                    online: this.currentTopic.online,
                    isBot: isBot
                };
            }
        }

        // Check contacts
        if (this.meTopic) {
            let userInfo = null;
            this.meTopic.contacts((sub) => {
                if (sub.topic === userId && sub.public?.fn) {
                    const name = sub.public.fn;
                    const isBot = this._isBotName(name);
                    this.cacheUserInfo(userId, name, isBot);
                    userInfo = {
                        id: userId,
                        name: name,
                        online: sub.online,
                        isBot: isBot
                    };
                }
            });
            if (userInfo) return userInfo;
        }

        // Check our local cache (fallback for users we've seen before)
        const cached = this._userCache[userId];
        if (cached?.name) {
            return {
                id: userId,
                name: cached.name,
                online: false, // Unknown
                isBot: cached.isBot || false
            };
        }

        return { id: userId, name: userId, isBot: false };
    }

    /**
     * Check if a name looks like a bot
     */
    _isBotName(name) {
        if (!name) return false;
        const lower = name.toLowerCase();
        return lower.includes('bot') || lower.includes('agent') || lower.includes('ai ');
    }

    /**
     * Convert photo data to a data URI for display
     * Tinode stores photos as base64 data, we need to prefix with data URI scheme
     */
    _photoToDataUri(photoData) {
        if (!photoData) return null;
        // If it's already a data URI or URL, return as-is
        if (photoData.startsWith('data:') || photoData.startsWith('http')) {
            return photoData;
        }
        // Assume JPEG for Tinode photos (most common)
        return `data:image/jpeg;base64,${photoData}`;
    }

    async searchUsers(query) {
        if (!query || query.trim().length === 0) {
            return [];
        }

        // Normalize: lowercase, split into words for multi-word matching
        const searchTerm = query.trim().toLowerCase();
        const searchWords = searchTerm.split(/\s+/).filter(w => w.length > 0);
        const primaryWord = searchWords[0]; // Use first word for fnd search

        const results = [];
        const seenTopics = new Set();

        // Helper: check if a name matches all search words
        const matchesSearch = (name) => {
            const nameLower = (name || '').toLowerCase();
            return searchWords.every(word => nameLower.includes(word));
        };

        // First, search local contacts
        if (this.meTopic) {
            this.meTopic.contacts((sub) => {
                const name = sub.public?.fn || sub.topic || '';
                if (matchesSearch(name) && sub.topic !== this.myUserId) {
                    seenTopics.add(sub.topic);
                    results.push({
                        topic: sub.topic,
                        name: name,
                        photo: this._photoToDataUri(sub.public?.photo?.data),
                        online: sub.online,
                        isBot: sub.public?.bot || this._isBotName(name),
                        description: sub.public?.agent?.description
                    });
                }
            });
        }

        // Then do server-side fnd search with first word (lowercase)
        try {
            // Get fresh fnd topic for each search to avoid cached results
            const fndTopic = this.client.getFndTopic();

            // Unsubscribe if previously subscribed (clears cache)
            if (fndTopic.isSubscribed()) {
                await fndTopic.leave(false);
            }

            // Subscribe fresh
            await fndTopic.subscribe();

            // Set search query - Tinode fnd does exact tag matching
            await fndTopic.setMeta({ desc: { public: primaryWord } });

            // Small delay for server to process the search
            await new Promise(r => setTimeout(r, 150));

            // Get results
            const fndResults = await new Promise((resolve) => {
                const found = [];
                let resolved = false;

                const finish = () => {
                    if (!resolved) {
                        resolved = true;
                        resolve(found);
                    }
                };

                const timeout = setTimeout(finish, 2000);

                fndTopic.onMetaSub = (sub) => {
                    const topic = sub.user || sub.topic;
                    const name = sub.public?.fn || topic;
                    const isBot = sub.public?.bot || this._isBotName(name);

                    // Cache user info for later lookups
                    if (topic && topic.startsWith('usr') && name !== topic) {
                        this.cacheUserInfo(topic, name, isBot);
                    }

                    // Filter: must match ALL search words (client-side filtering)
                    if (topic && topic !== this.myUserId && !seenTopics.has(topic) && matchesSearch(name)) {
                        seenTopics.add(topic);
                        found.push({
                            topic: topic,
                            name: name,
                            photo: this._photoToDataUri(sub.public?.photo?.data),
                            online: sub.online,
                            isBot: isBot,
                            description: sub.public?.agent?.description
                        });
                    }
                };

                fndTopic.onSubsUpdated = () => {
                    clearTimeout(timeout);
                    setTimeout(finish, 50);
                };

                fndTopic.getMeta(fndTopic.startMetaQuery().withSub().build())
                    .catch(() => finish());
            });

            results.push(...fndResults);

        } catch (err) {
            console.error('[Search] fnd error:', err);
        }

        return results;
    }

    async createGroup(name, description, workspaceId = null) {
        const topicName = 'new' + Math.random().toString(36).substr(2, 9);
        const topic = this.client.getTopic(topicName);

        topic.onData = (data) => {
            if (!data.from) return; // Skip incomplete messages
            this._emit('onMessage', {
                seq: data.seq,
                from: data.from,
                content: this._extractContent(data.content),
                rawContent: data.content,
                ts: data.ts,
                isOwn: data.from === this.myUserId
            });
        };

        // Build public metadata including workspace_id if provided
        const publicData = { fn: name };
        if (workspaceId) {
            publicData.workspace_id = workspaceId;
        }

        // First subscribe to create the topic
        await topic.subscribe(
            topic.startMetaQuery().withDesc().withSub().build()
        );

        // Then set the metadata separately
        await topic.setMeta({
            desc: {
                public: publicData,
                private: { comment: description }
            }
        });

        return topic.name;
    }

    /**
     * Create a workspace (Tinode grp topic with type=workspace metadata)
     * @param {string} name - Display name of the workspace
     * @param {string} slug - URL-friendly identifier
     * @param {string} description - Optional description
     * @returns {Promise<object>} - Workspace info including topic ID
     */
    async createWorkspace(name, slug, description = '') {
        const topicName = 'new' + Math.random().toString(36).substr(2, 9);
        const topic = this.client.getTopic(topicName);

        // First subscribe to create the topic
        await topic.subscribe(
            topic.startMetaQuery().withDesc().withSub().build()
        );

        // Set workspace metadata per TINODE_SCHEMA.md
        await topic.setMeta({
            desc: {
                public: {
                    fn: name,
                    type: 'workspace',
                    name: name,
                    slug: slug,
                    owner: this.myUserId
                },
                private: {
                    description: description
                }
            },
            tags: ['workspace', `slug:${slug}`]
        });

        console.log('[TinodeClient] Created workspace:', topic.name);

        const workspace = {
            id: topic.name,
            topic: topic.name,
            name: name,
            slug: slug,
            owner: this.myUserId,
            description: description
        };

        // Create default #main channel with welcome message
        try {
            const mainChannel = await this.createChannel(topic.name, 'main', 'Default channel');
            console.log('[TinodeClient] Created default #main channel:', mainChannel.id);

            // Send welcome message to the channel
            const channelTopic = this.client.getTopic(mainChannel.id);
            if (channelTopic.isSubscribed()) {
                await channelTopic.publish({
                    txt: `Welcome to ${name}! This is your workspace's default channel. Invite team members and start collaborating. You can create more channels, add AI agents, and manage everything from the workspace dropdown.`
                }, false);
            }
        } catch (err) {
            console.warn('[TinodeClient] Could not create default channel:', err);
            // Don't fail workspace creation if channel fails
        }

        return workspace;
    }

    /**
     * Get all workspaces the user is subscribed to
     * Filters grp topics where public.type === 'workspace'
     * @returns {Array} - List of workspace objects
     */
    getWorkspaces() {
        if (!this.meTopic) return [];

        const workspaces = [];

        this.meTopic.contacts((sub) => {
            // Only look at group topics
            if (!sub.topic?.startsWith('grp')) return;

            // Get public data from sub or cached topic
            let publicData = sub.public;
            if (!publicData) {
                const topic = this.client.getTopic(sub.topic);
                publicData = topic?.public;
            }

            // Check if it's a workspace
            if (publicData?.type === 'workspace') {
                workspaces.push({
                    id: sub.topic,
                    topic: sub.topic,
                    name: publicData.name || publicData.fn || sub.topic,
                    slug: publicData.slug,
                    owner: publicData.owner,
                    description: sub.private?.description || '',
                    updated: sub.updated,
                    touched: sub.touched
                });
            }
        });

        // Sort by most recently touched
        workspaces.sort((a, b) =>
            new Date(b.touched || b.updated) - new Date(a.touched || a.updated)
        );

        return workspaces;
    }

    /**
     * Get channels for a specific workspace
     * Filters grp topics where public.type === 'channel' and public.parent === workspaceId
     * @param {string} workspaceId - The workspace topic ID
     * @returns {Array} - List of channel objects
     */
    getWorkspaceChannels(workspaceId) {
        if (!this.meTopic) return [];

        const channels = [];

        this.meTopic.contacts((sub) => {
            if (!sub.topic?.startsWith('grp')) return;

            let publicData = sub.public;
            if (!publicData) {
                const topic = this.client.getTopic(sub.topic);
                publicData = topic?.public;
            }

            // Check if it's a channel belonging to this workspace
            if (publicData?.type === 'channel' && publicData?.parent === workspaceId) {
                channels.push({
                    id: sub.topic,
                    topic: sub.topic,
                    name: publicData.name || publicData.fn || sub.topic,
                    parent: publicData.parent,
                    updated: sub.updated,
                    touched: sub.touched,
                    unread: sub.seq - (sub.read || 0)
                });
            }
        });

        return channels;
    }

    /**
     * Convert an existing group to a workspace by adding workspace metadata
     * @param {string} topicId - The group topic ID to convert
     * @param {string} slug - URL-friendly identifier (optional, derived from name)
     * @returns {Promise<object>} - Updated workspace info
     */
    async convertGroupToWorkspace(topicId, slug = null) {
        const topic = this.client.getTopic(topicId);

        // Subscribe if not already
        const wasSubscribed = topic.isSubscribed();
        if (!wasSubscribed) {
            await topic.subscribe(
                topic.startMetaQuery().withDesc().withSub().build()
            );
        }

        // Get existing name from public.fn
        const existingName = topic.public?.fn || topicId;
        const workspaceSlug = slug || existingName.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');

        // Update metadata to include workspace type
        await topic.setMeta({
            desc: {
                public: {
                    fn: existingName,
                    type: 'workspace',
                    name: existingName,
                    slug: workspaceSlug,
                    owner: this.myUserId
                }
            },
            tags: ['workspace', `slug:${workspaceSlug}`]
        });

        console.log('[TinodeClient] Converted group to workspace:', topicId);

        return {
            id: topicId,
            topic: topicId,
            name: existingName,
            slug: workspaceSlug,
            owner: this.myUserId
        };
    }

    /**
     * Get groups that are NOT workspaces or channels (legacy groups)
     * These can be converted to workspaces
     * @returns {Array} - List of legacy group objects
     */
    getLegacyGroups() {
        if (!this.meTopic) return [];

        const legacyGroups = [];

        this.meTopic.contacts((sub) => {
            if (!sub.topic?.startsWith('grp')) return;

            let publicData = sub.public;
            if (!publicData) {
                const topic = this.client.getTopic(sub.topic);
                publicData = topic?.public;
            }

            // Check if it's NOT a workspace or channel
            const type = publicData?.type;
            if (!type || (type !== 'workspace' && type !== 'channel')) {
                legacyGroups.push({
                    id: sub.topic,
                    topic: sub.topic,
                    name: publicData?.fn || publicData?.name || sub.topic,
                    updated: sub.updated,
                    touched: sub.touched
                });
            }
        });

        return legacyGroups;
    }

    /**
     * Create a channel within a workspace
     * @param {string} workspaceId - Parent workspace topic ID
     * @param {string} name - Channel name
     * @param {string} description - Optional description
     * @returns {Promise<object>} - Channel info
     */
    async createChannel(workspaceId, name, description = '') {
        const topicName = 'new' + Math.random().toString(36).substr(2, 9);
        const topic = this.client.getTopic(topicName);

        await topic.subscribe(
            topic.startMetaQuery().withDesc().withSub().build()
        );

        await topic.setMeta({
            desc: {
                public: {
                    fn: name,
                    type: 'channel',
                    name: name,
                    parent: workspaceId
                },
                private: {
                    description: description
                },
                // Make channel open-join so workspace members can join without explicit invite
                defacs: {
                    auth: 'JRWPS',  // Authenticated users: Join, Read, Write, Presence, Share
                    anon: 'N'       // Anonymous: None
                }
            },
            tags: ['channel', `parent:${workspaceId}`]
        });

        console.log('[TinodeClient] Created channel:', topic.name, 'in workspace:', workspaceId);

        return {
            id: topic.name,
            topic: topic.name,
            name: name,
            parent: workspaceId,
            description: description
        };
    }

    /**
     * Find channels belonging to a workspace
     * @param {string} workspaceId - The workspace topic ID
     * @returns {Promise<Array>} - List of channel objects
     */
    async findWorkspaceChannels(workspaceId) {
        const channels = [];

        try {
            // Method 1: Check meTopic contacts for channels we're already subscribed to
            if (this.meTopic) {
                this.meTopic.contacts?.((contact) => {
                    if (!contact.topic?.startsWith('grp')) return;
                    const pub = contact.public;
                    if (pub?.type === 'channel' && pub?.parent === workspaceId) {
                        channels.push({
                            id: contact.topic,
                            name: pub?.name || pub?.fn || 'channel',
                            subscribed: true
                        });
                    }
                });
            }

            // Method 2: Use fnd to search for channels by parent tag
            if (channels.length === 0) {
                const fndTopic = this.client.getFndTopic();

                if (fndTopic.isSubscribed()) {
                    await fndTopic.leave(false);
                }

                await fndTopic.subscribe();
                await fndTopic.setMeta({ desc: { public: `parent:${workspaceId}` } });

                await new Promise((resolve) => {
                    const timeout = setTimeout(resolve, 1500);
                    fndTopic.onSubsUpdated = () => {
                        clearTimeout(timeout);
                        setTimeout(resolve, 100);
                    };
                    fndTopic.getMeta(fndTopic.startMetaQuery().withSub().build()).catch(() => {});
                });

                fndTopic.contacts?.((contact) => {
                    const topic = contact.topic || contact.user;
                    if (topic?.startsWith('grp') && contact.public?.type === 'channel') {
                        // Check if not already in list
                        if (!channels.find(c => c.id === topic)) {
                            channels.push({
                                id: topic,
                                name: contact.public?.name || contact.public?.fn || 'channel',
                                subscribed: false
                            });
                        }
                    }
                });
            }

            console.log('[TinodeClient] Found channels for workspace:', channels);
            return channels;
        } catch (err) {
            console.warn('[TinodeClient] Error finding channels:', err);
            return channels;
        }
    }

    /**
     * Join a channel (subscribe to it)
     * @param {string} channelId - The channel topic ID
     * @returns {Promise<boolean>} - Success status
     */
    async joinChannel(channelId) {
        try {
            const topic = this.client.getTopic(channelId);
            await topic.subscribe(
                topic.startMetaQuery().withDesc().withSub().withLaterData(50).build()
            );
            console.log('[TinodeClient] Joined channel:', channelId);
            return true;
        } catch (err) {
            console.warn('[TinodeClient] Failed to join channel:', channelId, err.message);
            return false;
        }
    }

    /**
     * Get agents (bot users) subscribed to a workspace
     * Agents are Tinode users with public.bot = true
     * @param {string} workspaceId - The workspace topic ID
     * @returns {Promise<Array>} - List of agent objects
     */
    async getWorkspaceAgents(workspaceId) {
        const agents = [];

        try {
            // Get or subscribe to the workspace topic
            let topic = this.client.getTopic(workspaceId);

            // Set up promise to wait for subscriber data
            const subsLoaded = new Promise((resolve) => {
                const originalHandler = topic.onSubsUpdated;
                topic.onSubsUpdated = (subs) => {
                    if (originalHandler) originalHandler.call(topic, subs);
                    resolve();
                };
            });

            // If not subscribed, subscribe with metadata request
            const wasSubscribed = topic.isSubscribed();
            if (!wasSubscribed) {
                await topic.subscribe(
                    topic.startMetaQuery().withSub().build()
                );
            } else {
                // Already subscribed - request fresh subscriber metadata
                await topic.getMeta(
                    topic.startMetaQuery().withSub().build()
                );
            }

            // Wait for subscriber data to be processed (with timeout)
            await Promise.race([
                subsLoaded,
                new Promise(r => setTimeout(r, 2000)) // 2s timeout
            ]);

            // Get subscribers and filter for bots, also cache all user info
            topic.subscribers((sub) => {
                const name = sub.public?.fn || sub.user;
                const isBot = sub.public?.bot === true;
                // Cache user info for later lookups
                if (name !== sub.user) {
                    this.cacheUserInfo(sub.user, name, isBot);
                }
                if (isBot) {
                    agents.push({
                        id: sub.user,
                        tinodeUserId: sub.user,
                        handle: sub.public.handle || sub.user,
                        displayName: sub.public.fn || sub.public.handle || 'Agent',
                        description: sub.public.description || '',
                        status: sub.online ? 'online' : 'offline',
                        isBot: true
                    });
                }
            });

            // Leave topic if we subscribed just for this query
            if (!wasSubscribed) {
                await topic.leave(false);
            }

            console.log('[TinodeClient] Found agents in workspace:', agents.length);
            return agents;

        } catch (error) {
            console.warn('[TinodeClient] Failed to get workspace agents:', error);
            return [];
        }
    }

    async uploadFile(file, onProgress) {
        const uploader = this.client.getLargeFileHelper();

        return new Promise((resolve, reject) => {
            uploader.upload(file,
                (progress) => onProgress?.(progress),
                (url) => resolve(url),
                (err) => reject(err)
            );
        });
    }

    getTopicMembers() {
        if (!this.currentTopic) return [];

        const members = [];
        this.currentTopic.subscribers((sub) => {
            const name = sub.public?.fn || sub.user;
            const isBot = this.isBot(sub.user);
            // Cache user info for later lookups
            if (name !== sub.user) {
                this.cacheUserInfo(sub.user, name, isBot);
            }
            members.push({
                id: sub.user,
                name: name,
                photo: this._photoToDataUri(sub.public?.photo?.data),
                online: sub.online,
                isBot: isBot
            });
        });
        return members;
    }

    /**
     * Check if a user ID belongs to a bot/agent
     */
    isBot(userId) {
        if (!userId) return false;
        // Check user's public data for bot indicator, or use naming convention
        const user = this.currentTopic?.userDesc(userId);
        if (user?.public?.bot) return true;
        // Fallback: check if name contains "bot" or "agent"
        const name = (user?.public?.fn || '').toLowerCase();
        return name.includes('bot') || name.includes('agent');
    }

    /**
     * Invite a user to the current group topic
     */
    async inviteMember(userId) {
        if (!this.currentTopic) {
            throw new Error('Not subscribed to any topic');
        }
        if (!this.currentTopic.name.startsWith('grp')) {
            throw new Error('Can only add members to group topics');
        }

        // Check permissions first
        if (!this.canPerform('invite')) {
            const perms = this.getMyPermissions();
            throw new Error(
                `You need admin or moderator rights to add members. ` +
                `Your role: ${perms?.role || 'Unknown'}`
            );
        }

        // Use setMeta to invite user with default access
        return this.currentTopic.setMeta({
            sub: {
                user: userId,
                mode: 'JRWPS' // Join, Read, Write, Presence, Share
            }
        });
    }

    /**
     * Remove a user from the current group topic
     */
    async removeMember(userId) {
        if (!this.currentTopic) {
            throw new Error('Not subscribed to any topic');
        }
        if (!this.currentTopic.name.startsWith('grp')) {
            throw new Error('Can only remove members from group topics');
        }

        // Check permissions first
        if (!this.canPerform('removeMember')) {
            const perms = this.getMyPermissions();
            throw new Error(
                `You need admin rights to remove members. ` +
                `Your role: ${perms?.role || 'Unknown'}`
            );
        }

        // Can't remove yourself this way
        if (userId === this.myUserId) {
            throw new Error('Use "Leave Group" to remove yourself');
        }

        // Set access mode to none to remove
        return this.currentTopic.setMeta({
            sub: {
                user: userId,
                mode: 'N' // None - removes access
            }
        });
    }

    async deleteMessage(seq, hard = false) {
        if (!this.currentTopic) throw new Error('Not subscribed');
        return this.currentTopic.delMessages(seq, hard);
    }

    /**
     * Edit an existing message
     * @param {number} seq - Message sequence number
     * @param {string} newContent - New message content
     */
    async editMessage(seq, newContent) {
        if (!this.currentTopic) throw new Error('Not subscribed');

        // Tinode SDK uses topic.note() for corrections/edits
        // The corrected message format uses {head: {replace: seq}} to indicate it's a correction
        return this.currentTopic.publish(
            { txt: newContent },
            false, // noEcho
            { replace: seq } // head - marks this as a replacement for seq
        );
    }

    /**
     * Add or toggle a reaction on a message
     * @param {number} seq - Message sequence number
     * @param {string} emoji - The reaction emoji
     */
    async addReaction(seq, emoji) {
        if (!this.currentTopic) throw new Error('Not subscribed');

        // Tinode stores reactions in message head
        // We'll use noteRecv to update the message metadata
        // Note: The actual implementation depends on Tinode server support
        // This uses the extra field to store reactions

        const topic = this.currentTopic;
        let msg = null;

        // Find the message
        topic.messages((m) => {
            if (m.seq === seq) {
                msg = m;
            }
        });

        if (!msg) throw new Error('Message not found');

        // Get current reactions
        const reactions = msg.head?.reactions || {};
        const myReactions = reactions[this.myUserId] || [];

        // Toggle reaction
        const idx = myReactions.indexOf(emoji);
        if (idx >= 0) {
            myReactions.splice(idx, 1);
        } else {
            myReactions.push(emoji);
        }

        // Update reactions
        if (myReactions.length > 0) {
            reactions[this.myUserId] = myReactions;
        } else {
            delete reactions[this.myUserId];
        }

        // Send as a note with reaction data
        // Note: Full implementation requires server-side support
        // For now, we store locally and emit the event
        if (!msg.head) msg.head = {};
        msg.head.reactions = reactions;

        return reactions;
    }

    /**
     * Set user presence/status
     * @param {string} status - 'online', 'away', 'dnd', 'offline'
     */
    async setPresence(status) {
        if (!this.meTopic) throw new Error('Not connected');

        // Map status to Tinode presence modes
        const modeMap = {
            'online': 'on',
            'away': 'away',
            'dnd': 'dnd',
            'offline': 'off'
        };

        const mode = modeMap[status] || 'on';

        // Set presence using meTopic note
        return this.meTopic.note('pres', mode);
    }

    disconnect() {
        this.client?.disconnect();
    }
}

window.TinodeClient = TinodeClient;
