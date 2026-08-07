import { useState, useMemo } from 'react'
import { ChevronRight, ChevronDown, Folder, FolderOpen, Plus, Pencil, Trash2, FolderInput } from 'lucide-react'
import type { BrowserGroupWithCount, BrowserGroupInput } from '../types'
import { createGroup, updateGroup, deleteGroup } from '../api'

interface GroupTreeNavProps {
  groups: BrowserGroupWithCount[]
  selectedGroupId: string | null
  onSelectGroup: (groupId: string | null) => void
  onRefresh: () => void
}

interface TreeNode extends BrowserGroupWithCount {
  children: TreeNode[]
  level: number
}

// 构建树形结构
function buildTree(groups: BrowserGroupWithCount[]): TreeNode[] {
  const map = new Map<string, TreeNode>()
  const roots: TreeNode[] = []

  // 初始化所有节点
  groups.forEach(g => {
    map.set(g.groupId, { ...g, children: [], level: 0 })
  })

  // 构建父子关系
  groups.forEach(g => {
    const node = map.get(g.groupId)!
    if (g.parentId && map.has(g.parentId)) {
      const parent = map.get(g.parentId)!
      node.level = parent.level + 1
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  })

  // 按 sortOrder 排序
  const sortNodes = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => a.sortOrder - b.sortOrder)
    nodes.forEach(n => sortNodes(n.children))
  }
  sortNodes(roots)

  return roots
}

export function GroupTreeNav({ groups, selectedGroupId, onSelectGroup, onRefresh }: GroupTreeNavProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [createParentId, setCreateParentId] = useState<string>('')
  const [newGroupName, setNewGroupName] = useState('')
  const [editingGroup, setEditingGroup] = useState<BrowserGroupWithCount | null>(null)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; group: BrowserGroupWithCount } | null>(null)

  const tree = useMemo(() => buildTree(groups), [groups])

  const toggleExpand = (groupId: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(groupId)) {
        next.delete(groupId)
      } else {
        next.add(groupId)
      }
      return next
    })
  }

  const handleCreate = async () => {
    if (!newGroupName.trim()) return
    const input: BrowserGroupInput = {
      groupName: newGroupName.trim(),
      parentId: createParentId,
      sortOrder: 0,
    }
    await createGroup(input)
    setShowCreateModal(false)
    setNewGroupName('')
    setCreateParentId('')
    onRefresh()
  }

  const handleRename = async () => {
    if (!editingGroup || !newGroupName.trim()) return
    const input: BrowserGroupInput = {
      groupName: newGroupName.trim(),
      parentId: editingGroup.parentId,
      sortOrder: editingGroup.sortOrder,
    }
    await updateGroup(editingGroup.groupId, input)
    setEditingGroup(null)
    setNewGroupName('')
    onRefresh()
  }

  const handleDelete = async (groupId: string) => {
    if (!confirm('确定删除此分组？子分组和实例将移动到父分组。')) return
    await deleteGroup(groupId)
    if (selectedGroupId === groupId) {
      onSelectGroup(null)
    }
    onRefresh()
  }

  const handleContextMenu = (e: React.MouseEvent, group: BrowserGroupWithCount) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, group })
  }

  const renderNode = (node: TreeNode) => {
    const isExpanded = expanded.has(node.groupId)
    const isSelected = selectedGroupId === node.groupId
    const hasChildren = node.children.length > 0

    return (
      <div key={node.groupId}>
        <div
          className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer rounded-[var(--radius-md)] hover:bg-[var(--bg-hover)] ${
            isSelected ? 'bg-[var(--accent-soft)] text-[var(--accent)]' : 'text-[var(--text-primary)]'
          }`}
          style={{ paddingLeft: `${node.level * 16 + 12}px` }}
          onClick={() => onSelectGroup(node.groupId)}
          onContextMenu={(e) => handleContextMenu(e, node)}
        >
          {hasChildren ? (
            <button
              className="p-0 hover:bg-[var(--bg-active)] rounded-[var(--radius-sm)] shrink-0"
              onClick={(e) => { e.stopPropagation(); toggleExpand(node.groupId) }}
            >
              {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
            </button>
          ) : null}
          {isExpanded && hasChildren ? (
            <FolderOpen className="w-4 h-4 text-[var(--accent)] shrink-0" />
          ) : (
            <Folder className="w-4 h-4 text-[var(--accent)] shrink-0" />
          )}
          <span className="flex-1 truncate text-sm">{node.groupName}</span>
          <span className="text-xs text-[var(--text-muted)]">{node.instanceCount}</span>
        </div>
        {isExpanded && node.children.map(child => renderNode(child))}
      </div>
    )
  }

  return (
    <div className="w-48 border-r border-[var(--border-subtle)] flex flex-col h-full">
      <div className="p-2 border-b border-[var(--border-subtle)] flex items-center justify-between">
        <span className="text-sm font-medium text-[var(--text-primary)]">分组</span>
        <button
          className="p-1 hover:bg-[var(--bg-hover)] rounded-[var(--radius-sm)] text-[var(--text-primary)]"
          onClick={() => { setCreateParentId(''); setShowCreateModal(true) }}
          title="新建分组"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto py-1">
        {/* 全部 */}
        <div
          className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer rounded-[var(--radius-md)] mx-1 hover:bg-[var(--bg-hover)] ${
            selectedGroupId === null ? 'bg-[var(--accent-soft)] text-[var(--accent)]' : 'text-[var(--text-primary)]'
          }`}
          onClick={() => onSelectGroup(null)}
        >
          <Folder className="w-4 h-4 text-[var(--text-muted)]" />
          <span className="flex-1 text-sm">全部</span>
        </div>

        {/* 未分组 */}
        <div
          className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer rounded-[var(--radius-md)] mx-1 hover:bg-[var(--bg-hover)] ${
            selectedGroupId === '__ungrouped__' ? 'bg-[var(--accent-soft)] text-[var(--accent)]' : 'text-[var(--text-primary)]'
          }`}
          onClick={() => onSelectGroup('__ungrouped__')}
        >
          <FolderInput className="w-4 h-4 text-[var(--text-muted)]" />
          <span className="flex-1 text-sm">未分组</span>
        </div>

        {/* 分组树 */}
        {tree.length > 0 && (
          <div className="mt-2 mx-1">
            <div className="px-2 py-1 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">我的分组</div>
            {tree.map(node => renderNode(node))}
          </div>
        )}
      </div>

      {/* 创建分组弹窗 */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[var(--z-modal)]" onClick={() => setShowCreateModal(false)}>
          <div className="bg-[var(--bg-overlay)] rounded-[var(--radius-lg)] shadow-[var(--shadow-lg)] p-4 w-80" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-medium mb-3 text-[var(--text-primary)]">新建分组</h3>
            <input
              type="text"
              className="w-full px-3 py-2 border border-[var(--border-subtle)] rounded-[var(--radius-md)] bg-[var(--bg-base)] text-[var(--text-primary)]"
              placeholder="分组名称"
              value={newGroupName}
              onChange={e => setNewGroupName(e.target.value)}
              autoFocus
            />
            {groups.length > 0 && (
              <select
                className="w-full mt-2 px-3 py-2 border border-[var(--border-subtle)] rounded-[var(--radius-md)] bg-[var(--bg-base)] text-[var(--text-primary)]"
                value={createParentId}
                onChange={e => setCreateParentId(e.target.value)}
              >
                <option value="">根级分组</option>
                {groups.map(g => (
                  <option key={g.groupId} value={g.groupId}>{g.groupName}</option>
                ))}
              </select>
            )}
            <div className="flex justify-end gap-2 mt-4">
              <button className="px-3 py-1.5 text-sm rounded-[var(--radius-md)] text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" onClick={() => setShowCreateModal(false)}>
                取消
              </button>
              <button className="px-3 py-1.5 text-sm bg-[var(--accent)] text-[var(--accent-contrast)] rounded-[var(--radius-md)] hover:bg-[var(--accent-hover)]" onClick={handleCreate}>
                创建
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 重命名弹窗 */}
      {editingGroup && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[var(--z-modal)]" onClick={() => setEditingGroup(null)}>
          <div className="bg-[var(--bg-overlay)] rounded-[var(--radius-lg)] shadow-[var(--shadow-lg)] p-4 w-80" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-medium mb-3 text-[var(--text-primary)]">重命名分组</h3>
            <input
              type="text"
              className="w-full px-3 py-2 border border-[var(--border-subtle)] rounded-[var(--radius-md)] bg-[var(--bg-base)] text-[var(--text-primary)]"
              placeholder="分组名称"
              value={newGroupName}
              onChange={e => setNewGroupName(e.target.value)}
              autoFocus
            />
            <div className="flex justify-end gap-2 mt-4">
              <button className="px-3 py-1.5 text-sm rounded-[var(--radius-md)] text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" onClick={() => setEditingGroup(null)}>
                取消
              </button>
              <button className="px-3 py-1.5 text-sm bg-[var(--accent)] text-[var(--accent-contrast)] rounded-[var(--radius-md)] hover:bg-[var(--accent-hover)]" onClick={handleRename}>
                保存
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 右键菜单 */}
      {contextMenu && (
        <div
          className="fixed bg-[var(--bg-overlay)] border border-[var(--border-subtle)] rounded-[var(--radius-md)] shadow-[var(--shadow-lg)] py-1 z-[var(--z-dropdown)] text-[var(--text-primary)]"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onClick={() => setContextMenu(null)}
        >
          <button
            className="w-full px-4 py-1.5 text-sm text-left hover:bg-[var(--bg-hover)] flex items-center gap-2"
            onClick={() => { setCreateParentId(contextMenu.group.groupId); setShowCreateModal(true) }}
          >
            <Plus className="w-4 h-4" /> 新建子分组
          </button>
          <button
            className="w-full px-4 py-1.5 text-sm text-left hover:bg-[var(--bg-hover)] flex items-center gap-2"
            onClick={() => { setNewGroupName(contextMenu.group.groupName); setEditingGroup(contextMenu.group) }}
          >
            <Pencil className="w-4 h-4" /> 重命名
          </button>
          <button
            className="w-full px-4 py-1.5 text-sm text-left hover:bg-[var(--bg-hover)] flex items-center gap-2 text-[var(--danger)]"
            onClick={() => handleDelete(contextMenu.group.groupId)}
          >
            <Trash2 className="w-4 h-4" /> 删除
          </button>
        </div>
      )}

      {/* 点击其他地方关闭右键菜单 */}
      {contextMenu && (
        <div className="fixed inset-0 z-[var(--z-overlay)]" onClick={() => setContextMenu(null)} />
      )}
    </div>
  )
}
