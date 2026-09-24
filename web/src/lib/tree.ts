export interface TreeNode {
  path: string
  name: string
  // null = not yet fetched (lazy); [] once fetched with no subfolders.
  children: TreeNode[] | null
  expanded: boolean
  loading: boolean
  error: boolean
}

export function makeTreeNode(path: string, name: string): TreeNode {
  return { path, name, children: null, expanded: false, loading: false, error: false }
}
