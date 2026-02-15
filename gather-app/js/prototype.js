/**
 * Prototype — Minimal view-switching logic
 * No frameworks. Plain DOM. Ready for React componentization.
 */

(function () {
    'use strict';

    // ===== State =====

    let currentChannel = 'general';
    let currentView = 'participants'; // 'participants' | 'agent-detail'
    let currentAgent = null;
    let deployStep = 1;

    // ===== Helpers =====

    function $(sel, ctx) { return (ctx || document).querySelector(sel); }
    function $$(sel, ctx) { return Array.from((ctx || document).querySelectorAll(sel)); }

    function hideAll(selector) {
        $$(selector).forEach(function (el) { el.style.display = 'none'; });
    }

    function show(el) {
        if (el) el.style.display = '';
    }

    function hide(el) {
        if (el) el.style.display = 'none';
    }

    // ===== Sidebar: Channel & DM switching =====

    function switchToChannel(name) {
        // Update sidebar active state
        $$('.sidebar-item').forEach(function (item) {
            item.classList.remove('active');
        });
        var target = $('[data-channel="' + name + '"]') || $('[data-dm="' + name + '"]');
        if (target) target.classList.add('active');

        // Remove unread badge on click
        if (target) {
            var badge = target.querySelector('.unread-badge');
            if (badge) badge.remove();
            target.classList.remove('has-unread');
        }

        // Show correct message list
        hideAll('.message-list');
        var listId = 'message-list-' + name;
        var list = document.getElementById(listId);
        if (list) {
            show(list);
            list.scrollTop = list.scrollHeight;
        }

        // Update channel header
        var headerName = $('.channel-header .channel-name');
        var headerTopic = $('.channel-header .channel-topic');
        var composer = $('.composer-input');

        var isDM = !!$('[data-dm="' + name + '"]');
        if (isDM) {
            var dmItem = $('[data-dm="' + name + '"]');
            var displayName = dmItem ? dmItem.querySelector('.item-name').textContent : name;
            headerName.innerHTML = displayName;
            headerTopic.textContent = 'Direct message';
            composer.placeholder = 'Message ' + displayName;
        } else {
            headerName.innerHTML = '<span class="hash">#</span> ' + name;
            headerTopic.textContent = channelTopics[name] || '';
            composer.placeholder = 'Message #' + name;
        }

        currentChannel = name;
    }

    var channelTopics = {
        general: 'Company-wide announcements and work-based matters',
        engineering: 'Code, architecture, and infrastructure',
        design: 'UI/UX design and assets',
        marketing: 'Campaigns, content, and launches',
        ops: 'Operations, monitoring, and incidents'
    };

    // Bind sidebar clicks
    $$('.sidebar-item[data-channel]').forEach(function (item) {
        item.addEventListener('click', function () {
            switchToChannel(item.dataset.channel);
        });
    });

    $$('.sidebar-item[data-dm]').forEach(function (item) {
        item.addEventListener('click', function () {
            switchToChannel('dm-' + item.dataset.dm);
        });
    });

    // ===== Detail Panel: Participants / Agent Detail =====

    function showParticipants() {
        show($('#participants-view'));
        hide($('#agent-detail-view'));
        $('#agent-detail-view').classList.remove('active');
        hide($('#detail-back'));
        $('#detail-title').textContent = 'Participants';
        currentView = 'participants';
        currentAgent = null;
    }

    function showAgentDetail(agentId) {
        hide($('#participants-view'));
        var detailView = $('#agent-detail-view');
        show(detailView);
        detailView.classList.add('active');

        // Show correct agent detail content
        $$('.agent-detail-content').forEach(function (el) { el.style.display = 'none'; });
        var agentContent = $('[data-agent-detail="' + agentId + '"]');
        if (agentContent) show(agentContent);

        show($('#detail-back'));
        var nameEl = agentContent ? agentContent.querySelector('.agent-detail-name') : null;
        $('#detail-title').textContent = nameEl ? nameEl.textContent : 'Agent';

        currentView = 'agent-detail';
        currentAgent = agentId;
    }

    // Agent items in participants
    $$('.agent-item[data-agent]').forEach(function (item) {
        item.addEventListener('click', function () {
            showAgentDetail(item.dataset.agent);
        });
    });

    // Back button
    $('#detail-back').addEventListener('click', showParticipants);

    // Close detail panel
    $$('[data-action="close-detail"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var panel = $('.detail-panel');
            panel.classList.toggle('hidden');
            var workspace = $('.workspace');
            workspace.classList.toggle('detail-collapsed');
        });
    });

    // Toggle detail panel from member count
    $$('[data-action="toggle-detail"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var panel = $('.detail-panel');
            var workspace = $('.workspace');
            if (panel.classList.contains('hidden')) {
                panel.classList.remove('hidden');
                workspace.classList.remove('detail-collapsed');
            } else {
                panel.classList.add('hidden');
                workspace.classList.add('detail-collapsed');
            }
        });
    });

    // ===== WebTop Fullscreen =====

    $$('[data-action="expand-webtop"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var overlay = $('#webtop-fullscreen');
            overlay.classList.add('active');
            // Set agent name in header
            var name = currentAgent === 'reviewclaw' ? 'ReviewClaw' : 'BuyClaw';
            $('#fs-agent-name').textContent = name;
        });
    });

    $('[data-action="exit-fullscreen"]').addEventListener('click', function () {
        $('#webtop-fullscreen').classList.remove('active');
    });

    // ESC to close fullscreen
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
            var fs = $('#webtop-fullscreen');
            if (fs.classList.contains('active')) {
                fs.classList.remove('active');
                return;
            }
            var settings = $('#settings-view');
            if (settings.classList.contains('active')) {
                settings.classList.remove('active');
                return;
            }
            var deployModal = $('#deploy-modal');
            if (deployModal.style.display !== 'none') {
                deployModal.style.display = 'none';
                resetDeployModal();
                return;
            }
        }
    });

    // ===== Deploy Agent Modal =====

    function resetDeployModal() {
        deployStep = 1;
        updateDeployStep();
    }

    function updateDeployStep() {
        // Update step dots
        $$('.step-dot').forEach(function (dot) {
            var step = parseInt(dot.dataset.stepDot);
            dot.classList.remove('active', 'completed');
            if (step === deployStep) dot.classList.add('active');
            if (step < deployStep) dot.classList.add('completed');
        });

        // Update connectors
        $$('.step-connector').forEach(function (conn) {
            var step = parseInt(conn.dataset.stepConn);
            conn.classList.remove('completed');
            if (step < deployStep) conn.classList.add('completed');
        });

        // Show active step
        $$('.deploy-step').forEach(function (step) {
            step.classList.remove('active');
        });
        var activeStep = $('[data-step="' + deployStep + '"]');
        if (activeStep) activeStep.classList.add('active');

        // Auto-advance from deploying step
        if (deployStep === 4) {
            setTimeout(function () {
                deployStep = 5;
                updateDeployStep();
            }, 2000);
        }
    }

    // Open deploy modal
    $$('[data-action="deploy-agent"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            resetDeployModal();
            $('#deploy-modal').style.display = 'flex';
        });
    });

    // Close deploy modal
    $$('[data-action="close-deploy"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            $('#deploy-modal').style.display = 'none';
            resetDeployModal();
        });
    });

    // Next step
    $$('[data-action="deploy-next"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            if (deployStep < 5) {
                deployStep++;
                updateDeployStep();
            }
        });
    });

    // Prev step
    $$('[data-action="deploy-prev"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            if (deployStep > 1) {
                deployStep--;
                updateDeployStep();
            }
        });
    });

    // Click outside modal to close
    $('#deploy-modal').addEventListener('click', function (e) {
        if (e.target === this) {
            this.style.display = 'none';
            resetDeployModal();
        }
    });

    // Type card selection
    $$('.type-card:not(.disabled)').forEach(function (card) {
        card.addEventListener('click', function () {
            $$('.type-card').forEach(function (c) { c.classList.remove('selected'); });
            card.classList.add('selected');
        });
    });

    // ===== Settings View =====

    // Open settings
    $$('[data-action="settings"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            $('#settings-view').classList.add('active');
        });
    });

    // Close settings
    $$('[data-action="close-settings"]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            $('#settings-view').classList.remove('active');
        });
    });

    // Settings tab switching
    $$('.settings-nav-item').forEach(function (item) {
        item.addEventListener('click', function (e) {
            e.preventDefault();
            var tab = item.dataset.settingsTab;

            // Update nav
            $$('.settings-nav-item').forEach(function (n) { n.classList.remove('active'); });
            item.classList.add('active');

            // Update sections
            $$('.settings-section').forEach(function (s) { s.classList.remove('active'); });
            var section = $('[data-settings-section="' + tab + '"]');
            if (section) section.classList.add('active');
        });
    });

    // ===== Auth Vault: Add/Cancel =====

    var vaultAddBtn = $('#vault-add-btn');
    var vaultAddForm = $('#vault-add-form');
    var vaultCancelBtn = $('#vault-cancel-btn');

    if (vaultAddBtn) {
        vaultAddBtn.addEventListener('click', function () {
            vaultAddForm.classList.add('active');
            vaultAddBtn.style.display = 'none';
        });
    }

    if (vaultCancelBtn) {
        vaultCancelBtn.addEventListener('click', function () {
            vaultAddForm.classList.remove('active');
            vaultAddBtn.style.display = '';
        });
    }

    // ===== Mode Toggle (Workspace / Network) =====

    $$('.mode-toggle-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var mode = btn.dataset.mode;
            $$('.mode-toggle-btn').forEach(function (b) { b.classList.remove('active'); });
            btn.classList.add('active');

            var workspace = $('.workspace');
            if (mode === 'network') {
                workspace.classList.add('mode-network');
            } else {
                workspace.classList.remove('mode-network');
            }
        });
    });

    // Network tab switching
    $$('.network-tab').forEach(function (tab) {
        tab.addEventListener('click', function () {
            var panel = tab.dataset.netTab;
            $$('.network-tab').forEach(function (t) { t.classList.remove('active'); });
            tab.classList.add('active');

            $$('.network-panel').forEach(function (p) { p.classList.remove('active'); });
            var target = $('[data-net-panel="' + panel + '"]');
            if (target) target.classList.add('active');
        });
    });

    // ===== Toggle switches =====

    $$('[data-toggle]').forEach(function (toggle) {
        toggle.addEventListener('click', function () {
            toggle.classList.toggle('on');
        });
    });

    // ===== Init =====

    // Scroll message lists to bottom
    $$('.message-list').forEach(function (list) {
        if (list.style.display !== 'none') {
            list.scrollTop = list.scrollHeight;
        }
    });

})();
