/**
 * Sidebar component - manages groups, DMs, and workspace agents list
 */

class Sidebar {
    constructor() {
        // DOM elements
        this.groupsList = document.getElementById('groups-list');
        this.dmsList = document.getElementById('dms-list');
        this.agentsList = document.getElementById('agents-list');
        this.groupsCount = document.getElementById('groups-count');
        this.dmsCount = document.getElementById('dms-count');
        this.agentsCount = document.getElementById('agents-count');
        this.groupsToggle = document.getElementById('groups-toggle');
        this.dmsToggle = document.getElementById('dms-toggle');
        this.agentsToggle = document.getElementById('agents-toggle');
        this.addGroupBtn = document.getElementById('add-group-btn');
        this.addDmBtn = document.getElementById('add-dm-btn');
        this.userAvatar = document.getElementById('user-avatar');
        this.userName = document.getElementById('user-name');
        this.userStatus = document.getElementById('user-status');
        this.userStatusDot = document.getElementById('user-status-dot');
        this.connectionDot = document.getElementById('connection-dot');
        this.connectionText = document.getElementById('connection-text');
        this.workspaceMenu = document.getElementById('workspace-menu');
        this.workspaceName = document.getElementById('workspace-name');
        this.teamOnlineList = document.getElementById('team-online-list');
        this.teamOnlineCount = document.getElementById('team-online-count');
        this.teamToggle = document.getElementById('team-toggle');

        // State
        this.selectedTopic = null;
        this.groupsCollapsed = false;
        this.dmsCollapsed = false;
        this.agentsCollapsed = false;
        this.teamCollapsed = false;
        this.workspaceMembers = []; // All members of current workspace
        this.currentWorkspace = null;
        this.workspaces = [];
        this.workspaceAgents = [];

        // Callbacks
        this.onTopicSelect = null;
        this.onAddGroup = null;
        this.onAddDm = null;
        this.onAddAgent = null;
        this.onWorkspaceChange = null;
        this.onAgentSelect = null;
        this.onSDKSettings = null;
        this.onWorkspaceSettings = null;
        this.onStatusChange = null;
        this.onInvite = null;
        this.onSignOut = null;

        // User status
        this.currentStatus = storage.get('userStatus') || 'online';

        this._init();
    }

    _init() {
        // Load collapsed state from storage
        this.groupsCollapsed = storage.getSidebarState('groups');
        this.dmsCollapsed = storage.getSidebarState('dms');
        this.agentsCollapsed = storage.getSidebarState('agents');
        this.teamCollapsed = storage.getSidebarState('team');
        this._updateCollapseState();

        // Toggle handlers
        this.groupsToggle?.addEventListener('click', () => {
            this.groupsCollapsed = !this.groupsCollapsed;
            storage.saveSidebarState('groups', this.groupsCollapsed);
            this._updateCollapseState();
        });

        this.dmsToggle?.addEventListener('click', () => {
            this.dmsCollapsed = !this.dmsCollapsed;
            storage.saveSidebarState('dms', this.dmsCollapsed);
            this._updateCollapseState();
        });

        this.agentsToggle?.addEventListener('click', () => {
            this.agentsCollapsed = !this.agentsCollapsed;
            storage.saveSidebarState('agents', this.agentsCollapsed);
            this._updateCollapseState();
        });

        this.teamToggle?.addEventListener('click', () => {
            this.teamCollapsed = !this.teamCollapsed;
            storage.saveSidebarState('team', this.teamCollapsed);
            this._updateCollapseState();
        });

        // Workspace menu handler
        this.workspaceMenu?.addEventListener('click', () => {
            this._showWorkspaceMenu();
        });

        // User profile/status handler
        document.getElementById('user-profile-btn')?.addEventListener('click', () => {
            this._showStatusMenu();
        });

        // Add buttons
        this.addGroupBtn?.addEventListener('click', () => {
            if (this.onAddGroup) this.onAddGroup();
        });

        this.addDmBtn?.addEventListener('click', () => {
            if (this.onAddDm) this.onAddDm();
        });

        document.getElementById('add-agent-btn')?.addEventListener('click', () => {
            if (this.onAddAgent) this.onAddAgent();
        });
    }

    _updateCollapseState() {
        // Update groups section
        if (this.groupsList) {
            this.groupsList.classList.toggle('hidden', this.groupsCollapsed);
        }
        const groupsArrow = this.groupsToggle?.querySelector('svg');
        if (groupsArrow) {
            groupsArrow.classList.toggle('-rotate-90', this.groupsCollapsed);
        }

        // Update DMs section
        if (this.dmsList) {
            this.dmsList.classList.toggle('hidden', this.dmsCollapsed);
        }
        const dmsArrow = this.dmsToggle?.querySelector('svg');
        if (dmsArrow) {
            dmsArrow.classList.toggle('-rotate-90', this.dmsCollapsed);
        }

        // Update Agents section
        if (this.agentsList) {
            this.agentsList.classList.toggle('hidden', this.agentsCollapsed);
        }
        const agentsArrow = this.agentsToggle?.querySelector('svg');
        if (agentsArrow) {
            agentsArrow.classList.toggle('-rotate-90', this.agentsCollapsed);
        }

        // Update Team Online section
        if (this.teamOnlineList) {
            this.teamOnlineList.classList.toggle('hidden', this.teamCollapsed);
        }
        const teamArrow = this.teamToggle?.querySelector('svg');
        if (teamArrow) {
            teamArrow.classList.toggle('-rotate-90', this.teamCollapsed);
        }
    }

    /**
     * Update the user profile section
     */
    setUser(user) {
        if (this.userName) {
            this.userName.textContent = user.name || user.id;
        }
        if (this.userAvatar) {
            if (user.photo) {
                this.userAvatar.innerHTML = `<img src="${user.photo}" class="w-full h-full rounded-lg object-cover" alt="${user.name}">`;
            } else {
                this.userAvatar.textContent = (user.name || user.id || '?').charAt(0).toUpperCase();
            }
        }
    }

    /**
     * Update connection status display
     */
    setConnectionStatus(connected, text) {
        if (this.connectionDot) {
            this.connectionDot.classList.remove('bg-slack-online', 'bg-slack-away', 'bg-slack-offline');
            this.connectionDot.classList.add(connected ? 'bg-slack-online' : 'bg-slack-offline');
        }
        if (this.connectionText) {
            this.connectionText.textContent = text || (connected ? 'Connected' : 'Disconnected');
        }
    }

    /**
     * Update groups and DMs lists
     */
    updateSubscriptions(data) {
        const groups = data?.groups || [];
        const dms = data?.dms || [];

        // Update counts
        if (this.groupsCount) {
            this.groupsCount.textContent = groups.length;
        }
        if (this.dmsCount) {
            this.dmsCount.textContent = dms.length;
        }

        // Check for groups with missing names (showing topic ID)
        // and request their metadata
        for (const group of groups) {
            if (!group || !group.topic) continue;
            if (group.name === group.topic && group.topic.startsWith('grp')) {
                this._fetchMissingGroupName(group.topic);
            }
        }

        // Render groups
        this._renderList(this.groupsList, groups, 'group');

        // Render DMs
        this._renderList(this.dmsList, dms, 'dm');
    }

    /**
     * Fetch group name for groups that don't have it cached
     */
    async _fetchMissingGroupName(topicName) {
        // Only fetch once per topic
        if (!this._fetchingTopics) this._fetchingTopics = new Set();
        if (this._fetchingTopics.has(topicName)) return;
        this._fetchingTopics.add(topicName);

        try {
            if (!window.app?.tinode?.client) return;

            const topic = window.app.tinode.client.getTopic(topicName);

            // Check if we already have the name cached
            if (topic?.public?.fn) {
                this._updateGroupName(topicName, topic.public.fn);
                return;
            }

            // Subscribe to get metadata
            const getQuery = topic.startMetaQuery().withDesc().withSub().build();

            // Set up handler for when we receive the description
            topic.onMeta = (meta) => {
                if (meta.desc?.public?.fn) {
                    this._updateGroupName(topicName, meta.desc.public.fn);
                }
            };

            await topic.subscribe(getQuery);

            // Wait a moment for meta to be processed
            await new Promise(r => setTimeout(r, 200));

            // Try to get name from public.fn
            const fetchedName = topic.public?.fn;
            if (fetchedName && fetchedName !== topicName) {
                this._updateGroupName(topicName, fetchedName);
            }

            // Leave the topic (we just wanted the metadata)
            if (window.app?.currentTopic !== topicName) {
                await topic.leave(false);
            }
        } catch (err) {
            console.warn(`[Sidebar] Could not fetch group name for ${topicName}:`, err.message);
        }
    }

    /**
     * Update a specific group's name in the sidebar
     */
    _updateGroupName(topic, name) {
        const item = this.groupsList?.querySelector(`[data-topic="${topic}"]`);
        if (item) {
            const nameSpan = item.querySelector('.truncate');
            if (nameSpan) {
                nameSpan.textContent = name;
            }
        }
    }

    _renderList(container, items, type) {
        if (!container) return;

        container.innerHTML = '';

        for (const item of items) {
            if (!item || !item.topic) continue;
            const el = this._createListItem(item, type);
            if (el) container.appendChild(el);
        }
    }

    _createListItem(item, type) {
        if (!item || !item.topic) return null;

        const div = document.createElement('div');
        const isSelected = this.selectedTopic === item.topic;

        div.className = `flex items-center space-x-2 px-4 py-1 cursor-pointer transition-colors ${
            isSelected
                ? 'bg-slack-sidebarActive text-white'
                : 'text-slack-sidebarText hover:bg-white/10'
        }`;
        div.dataset.topic = item.topic;

        // Icon/Avatar
        const iconDiv = document.createElement('div');
        iconDiv.className = 'flex-shrink-0';

        if (type === 'dm') {
            // User avatar with online status - different style for bots
            if (item.isBot) {
                iconDiv.innerHTML = `
                    <div class="relative">
                        <div class="w-5 h-5 rounded bg-purple-500 flex items-center justify-center text-xs text-white">
                            <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                        </div>
                        <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full border border-slack-sidebar ${
                            item.online ? 'bg-slack-online' : 'bg-transparent border-slack-sidebarText'
                        }"></span>
                    </div>
                `;
            } else {
                iconDiv.innerHTML = `
                    <div class="relative">
                        <div class="w-5 h-5 rounded bg-slack-sidebarActive flex items-center justify-center text-xs text-white">
                            ${item.photo
                                ? `<img src="${item.photo}" class="w-full h-full rounded object-cover">`
                                : (item.name || item.topic).charAt(0).toUpperCase()
                            }
                        </div>
                        <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full border border-slack-sidebar ${
                            item.online ? 'bg-slack-online' : 'bg-transparent border-slack-sidebarText'
                        }"></span>
                    </div>
                `;
            }
        } else {
            // Group hash icon
            iconDiv.innerHTML = `
                <span class="text-sm ${isSelected ? 'text-white' : 'text-slack-sidebarText'}">#</span>
            `;
        }

        // Name
        const nameSpan = document.createElement('span');
        nameSpan.className = 'truncate text-sm flex-1';
        nameSpan.textContent = item.name || item.topic;

        // Unread badge
        const badgeSpan = document.createElement('span');
        if (item.unread > 0) {
            badgeSpan.className = 'flex-shrink-0 bg-red-500 text-white text-xs rounded-full px-1.5 min-w-[1.25rem] text-center';
            badgeSpan.textContent = item.unread > 99 ? '99+' : item.unread;
        }

        div.appendChild(iconDiv);
        div.appendChild(nameSpan);
        if (item.unread > 0) {
            div.appendChild(badgeSpan);
        }

        // Click handler
        div.addEventListener('click', () => {
            this.selectTopic(item.topic);
            if (this.onTopicSelect) {
                this.onTopicSelect(item.topic, item);
            }
        });

        return div;
    }

    /**
     * Select a topic
     */
    selectTopic(topic) {
        this.selectedTopic = topic;

        // Update all items
        const allItems = document.querySelectorAll('#groups-list > div, #dms-list > div');
        allItems.forEach(el => {
            const isSelected = el.dataset.topic === topic;
            el.classList.toggle('bg-slack-sidebarActive', isSelected);
            el.classList.toggle('text-white', isSelected);
            el.classList.toggle('text-slack-sidebarText', !isSelected);
            el.classList.toggle('hover:bg-white/10', !isSelected);
        });
    }

    /**
     * Update unread count for a topic
     */
    updateUnread(topic, count) {
        const item = document.querySelector(`[data-topic="${topic}"]`);
        if (!item) return;

        let badge = item.querySelector('.bg-red-500');

        if (count > 0) {
            if (!badge) {
                badge = document.createElement('span');
                badge.className = 'flex-shrink-0 bg-red-500 text-white text-xs rounded-full px-1.5 min-w-[1.25rem] text-center';
                item.appendChild(badge);
            }
            badge.textContent = count > 99 ? '99+' : count;
        } else if (badge) {
            badge.remove();
        }
    }

    /**
     * Update online status for a user/topic
     */
    updatePresence(userId, online) {
        // Update DMs list
        const dmItem = document.querySelector(`#dms-list [data-topic="${userId}"]`);
        if (dmItem) {
            const statusDot = dmItem.querySelector('.rounded-full');
            if (statusDot) {
                statusDot.classList.toggle('bg-slack-online', online);
                statusDot.classList.toggle('bg-transparent', !online);
            }
        }

        // Update agents list - check by tinodeUserId
        const agentItem = document.querySelector(`[data-tinode-user-id="${userId}"]`);
        if (agentItem) {
            const statusDot = agentItem.querySelector('.rounded-full');
            if (statusDot) {
                statusDot.classList.toggle('bg-slack-online', online);
                statusDot.classList.toggle('bg-slack-offline', !online);
            }
        }

        // Also update the internal agent state
        const agent = this.workspaceAgents.find(a => a.tinodeUserId === userId);
        if (agent) {
            agent.status = online ? 'online' : 'offline';
        }

        // Update workspace members online status and re-render
        const member = this.workspaceMembers.find(m => m.id === userId);
        if (member) {
            member.online = online;
            this._renderTeamOnline();
        }
    }

    /**
     * Set workspace members (called when switching workspaces or loading members)
     */
    setWorkspaceMembers(members) {
        this.workspaceMembers = members || [];
        this._renderTeamOnline();
    }

    /**
     * Render the team list (all members with online/offline status)
     */
    _renderTeamOnline() {
        if (!this.teamOnlineList) return;

        // Get all team members excluding current user and bots
        const myUserId = window.tinodeClient?.myUserId;
        const teamMembers = this.workspaceMembers.filter(m =>
            m.id !== myUserId && !m.isBot
        );

        // Sort: online first, then by name
        teamMembers.sort((a, b) => {
            if (a.online && !b.online) return -1;
            if (!a.online && b.online) return 1;
            return (a.name || a.id).localeCompare(b.name || b.id);
        });

        // Update count (show total, not just online)
        if (this.teamOnlineCount) {
            this.teamOnlineCount.textContent = teamMembers.length;
        }

        // Clear and render list
        this.teamOnlineList.innerHTML = '';

        if (teamMembers.length === 0) {
            const emptyDiv = document.createElement('div');
            emptyDiv.className = 'px-4 py-2 text-xs text-slack-sidebarText/60 italic';
            emptyDiv.textContent = 'No team members yet';
            this.teamOnlineList.appendChild(emptyDiv);
            return;
        }

        teamMembers.forEach(member => {
            const div = document.createElement('div');
            div.className = 'flex items-center space-x-2 px-4 py-1 cursor-pointer transition-colors text-slack-sidebarText hover:bg-white/10';
            div.dataset.userId = member.id;

            // Status dot: green if online, hollow circle if offline
            const statusClass = member.online
                ? 'bg-slack-online'
                : 'bg-transparent border-slack-sidebarText';

            div.innerHTML = `
                <div class="relative flex-shrink-0">
                    <div class="w-5 h-5 rounded bg-slack-sidebarActive flex items-center justify-center text-xs text-white">
                        ${member.photo
                            ? `<img src="${member.photo}" class="w-full h-full rounded object-cover">`
                            : (member.name || member.id).charAt(0).toUpperCase()
                        }
                    </div>
                    <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full border border-slack-sidebar ${statusClass}"></span>
                </div>
                <span class="truncate text-sm flex-1">${member.name || member.id}</span>
            `;

            // Click to start DM
            div.addEventListener('click', () => {
                if (this.onTopicSelect) {
                    this.onTopicSelect(member.id, {
                        name: member.name,
                        isBot: false,
                        online: member.online
                    });
                }
            });

            this.teamOnlineList.appendChild(div);
        });
    }

    /**
     * Set available workspaces and current workspace
     */
    setWorkspaces(workspaces, currentId = null) {
        this.workspaces = workspaces;
        if (currentId) {
            this.currentWorkspace = workspaces.find(w => w.id === currentId);
        } else if (workspaces.length > 0) {
            this.currentWorkspace = workspaces[0];
        }
        this._updateWorkspaceDisplay();
    }

    /**
     * Set current workspace and notify listeners
     */
    setCurrentWorkspace(workspaceId) {
        const workspace = this.workspaces.find(w => w.id === workspaceId);
        if (workspace) {
            this.currentWorkspace = workspace;
            this._updateWorkspaceDisplay();
            if (this.onWorkspaceChange) {
                this.onWorkspaceChange(workspace);
            }
        }
    }

    _updateWorkspaceDisplay() {
        if (this.workspaceName) {
            if (this.currentWorkspace) {
                this.workspaceName.textContent = this.currentWorkspace.name || this.currentWorkspace.slug || 'Workspace';
            } else if (this.workspaces.length === 0) {
                // No workspaces yet - show app name with hint
                this.workspaceName.textContent = 'Gather.is';
            }
        }
    }

    _showWorkspaceMenu() {
        // Create or show workspace dropdown
        const existingMenu = document.getElementById('workspace-dropdown');
        if (existingMenu) {
            existingMenu.remove();
            return;
        }

        const dropdown = document.createElement('div');
        dropdown.id = 'workspace-dropdown';
        dropdown.className = 'absolute left-4 top-14 w-56 bg-gray-800 rounded-lg shadow-xl z-50 py-2';

        // Add workspaces
        this.workspaces.forEach(ws => {
            const item = document.createElement('button');
            item.className = `w-full px-4 py-2 text-left text-sm hover:bg-gray-700 ${
                ws.id === this.currentWorkspace?.id ? 'text-white bg-gray-700' : 'text-gray-300'
            }`;
            item.innerHTML = `
                <div class="flex items-center justify-between">
                    <span>${ws.name}</span>
                    ${ws.id === this.currentWorkspace?.id ? '<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/></svg>' : ''}
                </div>
                <span class="text-xs text-gray-500">${ws.role}</span>
            `;
            item.addEventListener('click', () => {
                this.setCurrentWorkspace(ws.id);
                dropdown.remove();
            });
            dropdown.appendChild(item);
        });

        // Add divider and create option
        if (this.workspaces.length > 0) {
            const divider = document.createElement('div');
            divider.className = 'border-t border-gray-700 my-2';
            dropdown.appendChild(divider);
        }

        // Workspace Settings button (only if workspace selected)
        if (this.currentWorkspace) {
            const settingsBtn = document.createElement('button');
            settingsBtn.className = 'w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700';
            settingsBtn.innerHTML = `
                <div class="flex items-center space-x-2">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
                    </svg>
                    <span>Workspace Settings</span>
                </div>
            `;
            settingsBtn.addEventListener('click', () => {
                dropdown.remove();
                if (this.onWorkspaceSettings) {
                    this.onWorkspaceSettings(this.currentWorkspace);
                }
            });
            dropdown.appendChild(settingsBtn);

            // Invite People button
            const inviteBtn = document.createElement('button');
            inviteBtn.className = 'w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700';
            inviteBtn.innerHTML = `
                <div class="flex items-center space-x-2">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z"/>
                    </svg>
                    <span>Invite People</span>
                </div>
            `;
            inviteBtn.addEventListener('click', () => {
                dropdown.remove();
                if (this.onInvite) {
                    this.onInvite(this.currentWorkspace);
                }
            });
            dropdown.appendChild(inviteBtn);
        }

        // SDK Settings button
        const sdkBtn = document.createElement('button');
        sdkBtn.className = 'w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700';
        sdkBtn.innerHTML = `
            <div class="flex items-center space-x-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/>
                </svg>
                <span>SDK Settings</span>
            </div>
        `;
        sdkBtn.addEventListener('click', () => {
            dropdown.remove();
            if (this.onSDKSettings) {
                this.onSDKSettings(this.currentWorkspace);
            }
        });
        dropdown.appendChild(sdkBtn);

        const createBtn = document.createElement('button');
        createBtn.className = 'w-full px-4 py-2 text-left text-sm text-gray-300 hover:bg-gray-700';
        createBtn.innerHTML = `
            <div class="flex items-center space-x-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/>
                </svg>
                <span>Create Workspace</span>
            </div>
        `;
        createBtn.addEventListener('click', () => {
            dropdown.remove();
            this._showCreateWorkspaceModal();
        });
        dropdown.appendChild(createBtn);

        // Add to DOM
        this.workspaceMenu.parentElement.appendChild(dropdown);

        // Close on click outside
        setTimeout(() => {
            document.addEventListener('click', function closeDropdown(e) {
                if (!dropdown.contains(e.target) && !document.getElementById('workspace-menu').contains(e.target)) {
                    dropdown.remove();
                    document.removeEventListener('click', closeDropdown);
                }
            });
        }, 0);
    }

    _showCreateWorkspaceModal() {
        const modal = document.createElement('div');
        modal.className = 'fixed inset-0 bg-black/50 flex items-center justify-center z-50';
        modal.innerHTML = `
            <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md mx-4 p-6">
                <h3 class="text-lg font-bold text-gray-900 dark:text-white mb-4">Create a workspace</h3>
                <form id="create-workspace-form">
                    <div class="mb-4">
                        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Name</label>
                        <input type="text" id="workspace-name-input"
                            class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-slack-accent dark:bg-gray-700 dark:text-white"
                            placeholder="e.g. My Team">
                    </div>
                    <div class="mb-4">
                        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Slug (URL-friendly)</label>
                        <input type="text" id="workspace-slug-input"
                            class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-slack-accent dark:bg-gray-700 dark:text-white"
                            placeholder="e.g. my-team">
                    </div>
                    <div class="flex justify-end space-x-3">
                        <button type="button" id="cancel-workspace-btn" class="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
                            Cancel
                        </button>
                        <button type="submit" class="px-4 py-2 bg-slack-accent hover:bg-blue-700 text-white rounded-lg">
                            Create
                        </button>
                    </div>
                </form>
            </div>
        `;

        document.body.appendChild(modal);

        // Auto-generate slug from name
        const nameInput = modal.querySelector('#workspace-name-input');
        const slugInput = modal.querySelector('#workspace-slug-input');
        nameInput.addEventListener('input', () => {
            slugInput.value = nameInput.value.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
        });

        // Cancel button
        modal.querySelector('#cancel-workspace-btn').addEventListener('click', () => modal.remove());

        // Form submit
        modal.querySelector('#create-workspace-form').addEventListener('submit', async (e) => {
            e.preventDefault();
            const name = nameInput.value.trim();
            const slug = slugInput.value.trim();
            if (!name || !slug) return;

            try {
                const workspace = await window.authManager.createWorkspace(name, slug);
                this.workspaces.push(workspace);
                this.setCurrentWorkspace(workspace.id);
                modal.remove();
                notifications.success(`Workspace "${name}" created!`);
            } catch (err) {
                notifications.error(`Failed to create workspace: ${err.message}`);
            }
        });
    }

    /**
     * Update workspace agents list
     */
    updateAgents(agents) {
        this.workspaceAgents = agents;

        if (this.agentsCount) {
            this.agentsCount.textContent = agents.length;
        }

        if (!this.agentsList) return;

        this.agentsList.innerHTML = '';

        agents.forEach(agent => {
            const el = this._createAgentItem(agent);
            this.agentsList.appendChild(el);
        });
    }

    _createAgentItem(agent) {
        const div = document.createElement('div');
        const isSelected = this.selectedTopic === agent.tinodeUserId;

        div.className = `flex items-center space-x-2 px-4 py-1 cursor-pointer transition-colors ${
            isSelected
                ? 'bg-slack-sidebarActive text-white'
                : 'text-slack-sidebarText hover:bg-white/10'
        }`;
        div.dataset.agentHandle = agent.handle;
        div.dataset.tinodeUserId = agent.tinodeUserId || '';

        // Bot icon with status indicator
        const iconDiv = document.createElement('div');
        iconDiv.className = 'flex-shrink-0';
        iconDiv.innerHTML = `
            <div class="relative">
                <div class="w-5 h-5 rounded bg-purple-500 flex items-center justify-center text-xs text-white">
                    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                    </svg>
                </div>
                <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full border border-slack-sidebar ${
                    (agent.status === 'online' || agent.status === 'running' || agent.status === 'dev') ? 'bg-slack-online' : 'bg-slack-offline'
                }"></span>
            </div>
        `;

        // Name with [DEV] badge if applicable
        const nameSpan = document.createElement('span');
        nameSpan.className = 'truncate text-sm flex-1';
        const isDev = agent.displayName?.includes('[DEV]');
        nameSpan.innerHTML = `
            @${agent.handle}
            ${isDev ? '<span class="ml-1 text-xs text-yellow-400">[DEV]</span>' : ''}
        `;

        div.appendChild(iconDiv);
        div.appendChild(nameSpan);

        // Click handler - open DM with agent
        div.addEventListener('click', () => {
            if (agent.tinodeUserId) {
                this.selectTopic(agent.tinodeUserId);
                if (this.onTopicSelect) {
                    this.onTopicSelect(agent.tinodeUserId, {
                        name: agent.displayName,
                        isBot: true,
                        agent: agent
                    });
                }
            }
            if (this.onAgentSelect) {
                this.onAgentSelect(agent);
            }
        });

        return div;
    }

    /**
     * Get current workspace ID
     */
    getCurrentWorkspaceId() {
        return this.currentWorkspace?.id;
    }

    /**
     * Show status selection menu
     */
    _showStatusMenu() {
        const existingMenu = document.getElementById('status-dropdown');
        if (existingMenu) {
            existingMenu.remove();
            return;
        }

        const dropdown = document.createElement('div');
        dropdown.id = 'status-dropdown';
        dropdown.className = 'absolute left-4 top-28 w-52 bg-gray-800 rounded-lg shadow-xl z-50 py-2';

        const statuses = [
            { id: 'online', label: 'Active', color: 'bg-slack-online', description: 'You appear as active' },
            { id: 'away', label: 'Away', color: 'bg-slack-away', description: 'You appear as away' },
            { id: 'dnd', label: 'Do Not Disturb', color: 'bg-red-500', description: 'Pause notifications' },
            { id: 'offline', label: 'Invisible', color: 'bg-slack-offline', description: 'Appear offline' }
        ];

        statuses.forEach(status => {
            const item = document.createElement('button');
            item.className = `w-full px-4 py-2 text-left hover:bg-gray-700 flex items-center space-x-3 ${
                this.currentStatus === status.id ? 'bg-gray-700' : ''
            }`;
            item.innerHTML = `
                <span class="w-3 h-3 rounded-full ${status.color}"></span>
                <div class="flex-1">
                    <div class="text-sm text-white">${status.label}</div>
                    <div class="text-xs text-gray-400">${status.description}</div>
                </div>
                ${this.currentStatus === status.id ? `
                <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
                </svg>
                ` : ''}
            `;
            item.addEventListener('click', () => {
                this.setStatus(status.id);
                dropdown.remove();
            });
            dropdown.appendChild(item);
        });

        // Divider
        const divider = document.createElement('div');
        divider.className = 'border-t border-gray-700 my-2';
        dropdown.appendChild(divider);

        // Sign Out button
        const signOutBtn = document.createElement('button');
        signOutBtn.className = 'w-full px-4 py-2 text-left hover:bg-gray-700 flex items-center space-x-3 text-red-400';
        signOutBtn.innerHTML = `
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/>
            </svg>
            <span class="text-sm">Sign Out</span>
        `;
        signOutBtn.addEventListener('click', async () => {
            dropdown.remove();
            if (this.onSignOut) {
                this.onSignOut();
            }
        });
        dropdown.appendChild(signOutBtn);

        // Add to DOM
        const sidebar = document.getElementById('sidebar');
        sidebar?.appendChild(dropdown);

        // Close on click outside
        setTimeout(() => {
            document.addEventListener('click', function closeDropdown(e) {
                if (!dropdown.contains(e.target) && !document.getElementById('user-profile-btn')?.contains(e.target)) {
                    dropdown.remove();
                    document.removeEventListener('click', closeDropdown);
                }
            });
        }, 0);
    }

    /**
     * Set user status
     */
    setStatus(status) {
        this.currentStatus = status;
        storage.set('userStatus', status);

        // Update UI
        this._updateStatusDisplay(status);

        // Notify listeners
        if (this.onStatusChange) {
            this.onStatusChange(status);
        }
    }

    /**
     * Update status display in sidebar
     */
    _updateStatusDisplay(status) {
        const statusDot = this.userStatusDot;
        const statusText = this.userStatus;

        if (statusDot) {
            statusDot.classList.remove('bg-slack-online', 'bg-slack-away', 'bg-red-500', 'bg-slack-offline');
            switch (status) {
                case 'online':
                    statusDot.classList.add('bg-slack-online');
                    break;
                case 'away':
                    statusDot.classList.add('bg-slack-away');
                    break;
                case 'dnd':
                    statusDot.classList.add('bg-red-500');
                    break;
                case 'offline':
                    statusDot.classList.add('bg-slack-offline');
                    break;
            }
        }

        if (statusText) {
            const labels = {
                'online': 'Active',
                'away': 'Away',
                'dnd': 'Do Not Disturb',
                'offline': 'Invisible'
            };
            statusText.textContent = labels[status] || 'Active';
        }
    }

    /**
     * Initialize status from storage
     */
    initStatus() {
        this._updateStatusDisplay(this.currentStatus);
    }
}

// Export
window.Sidebar = Sidebar;
