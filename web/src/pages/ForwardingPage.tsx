import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api, ForwardRule, ForwardType, Agent } from '@/api/client'
import { formatBytes, formatSpeed } from '@/lib/utils'
import { Plus, Trash2, ArrowRight, RefreshCw, Gauge } from 'lucide-react'

export function ForwardingPage() {
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  
  const { data: rules, isLoading } = useQuery({
    queryKey: ['forward-rules'],
    queryFn: api.getForwardRules,
    refetchInterval: 5000,
  })

  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: api.getAgents,
  })

  const createMutation = useMutation({
    mutationFn: api.createForwardRule,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['forward-rules'] })
      setDialogOpen(false)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: api.deleteForwardRule,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['forward-rules'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateForwardRule(id, { enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['forward-rules'] })
    },
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="w-6 h-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Card className="bg-card/50 border-border/50">
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-lg">端口转发规则</CardTitle>
          <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="w-4 h-4" />
                添加规则
              </Button>
            </DialogTrigger>
            <DialogContent>
              <CreateRuleDialog
                agents={agents || []}
                onSubmit={(rule) => createMutation.mutate(rule)}
                isLoading={createMutation.isPending}
              />
            </DialogContent>
          </Dialog>
        </CardHeader>
        <CardContent>
          {rules && rules.length > 0 ? (
            <div className="space-y-3">
              {rules.map((rule) => (
                <RuleCard
                  key={rule.id}
                  rule={rule}
                  agents={agents || []}
                  onToggle={(enabled) => toggleMutation.mutate({ id: rule.id, enabled })}
                  onDelete={() => deleteMutation.mutate(rule.id)}
                />
              ))}
            </div>
          ) : (
            <div className="text-center py-12 text-muted-foreground">
              <ArrowRight className="w-12 h-12 mx-auto mb-4 opacity-50" />
              <p>暂无转发规则</p>
              <p className="text-sm mt-2">点击上方按钮添加端口转发规则</p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function RuleCard({
  rule,
  agents,
  onToggle,
  onDelete,
}: {
  rule: ForwardRule
  agents: Agent[]
  onToggle: (enabled: boolean) => void
  onDelete: () => void
}) {
  const sourceAgent = agents.find(a => a.id === rule.sourceAgentId)
  const targetAgent = agents.find(a => a.id === rule.targetAgentId)

  // Determine display based on forward type
  const getTypeLabel = () => {
    switch (rule.type) {
      case 'cloud-direct':
      case 'cloud-self':
        return '云端直连'
      case 'cloud-agent':
      case 'remote':
        return '云端→客户端'
      case 'agent-cloud':
        return '客户端→云端'
      case 'agent-agent':
      case 'local':
      case 'p2p':
        return '客户端→客户端'
      default:
        return rule.type
    }
  }

  // Determine the listen side display
  const getListenSide = () => {
    switch (rule.type) {
      case 'cloud-direct':
      case 'cloud-self':
      case 'cloud-agent':
      case 'remote':
        return `Cloud:${rule.listenPort}`
      case 'agent-cloud':
      case 'agent-agent':
      case 'local':
      case 'p2p':
        return `${sourceAgent?.name || rule.sourceAgentId || 'Unknown'}:${rule.listenPort}`
      default:
        return `${rule.listenPort}`
    }
  }

  // Determine the target side display
  const getTargetSide = () => {
    switch (rule.type) {
      case 'cloud-direct':
      case 'cloud-self':
      case 'agent-cloud':
        return `${rule.targetHost}:${rule.targetPort}`
      case 'cloud-agent':
      case 'remote':
      case 'agent-agent':
      case 'local':
      case 'p2p':
        return `${targetAgent?.name || rule.targetAgentId}→${rule.targetHost}:${rule.targetPort}`
      default:
        return `${rule.targetHost}:${rule.targetPort}`
    }
  }

  return (
    <div className="flex items-center justify-between p-4 rounded-lg border border-border/50 bg-background/50">
      <div className="flex items-center gap-4">
        <Switch
          checked={rule.enabled}
          onCheckedChange={onToggle}
        />
        <div className="flex items-center gap-2">
          <Badge variant={rule.protocol === 'tcp' ? 'default' : 'secondary'}>
            {rule.protocol.toUpperCase()}
          </Badge>
          <Badge variant="outline" className="text-xs">
            {getTypeLabel()}
          </Badge>
          <span className="text-sm font-mono">
            {getListenSide()}
          </span>
          <ArrowRight className="w-4 h-4 text-muted-foreground" />
          <span className="text-sm font-mono">
            {getTargetSide()}
          </span>
        </div>
      </div>
      <div className="flex items-center gap-4">
        <div className="text-right text-xs text-muted-foreground">
          <div className="flex items-center gap-1">
            <span>流量: {formatBytes(rule.trafficUsed)}</span>
            {rule.trafficLimit > 0 && (
              <span className="text-muted-foreground/60">/ {formatBytes(rule.trafficLimit)}</span>
            )}
          </div>
          {rule.rateLimit > 0 && (
            <div className="flex items-center gap-1">
              <Gauge className="w-3 h-3" />
              <span>限速: {formatSpeed(rule.rateLimit)}</span>
            </div>
          )}
        </div>
        <Badge variant={rule.enabled ? 'success' : 'outline'}>
          {rule.enabled ? '运行中' : '已停止'}
        </Badge>
        <Button variant="ghost" size="icon" onClick={onDelete}>
          <Trash2 className="w-4 h-4 text-destructive" />
        </Button>
      </div>
    </div>
  )
}

interface CreateRuleForm {
  name: string
  type: ForwardType
  protocol: 'tcp' | 'udp'
  sourceAgentId: string
  listenPort: string
  targetAgentId: string
  targetHost: string
  targetPort: string
  rateLimit: string      // MB/s, empty = unlimited
  trafficLimit: string   // GB, empty = unlimited
}

function CreateRuleDialog({
  agents,
  onSubmit,
  isLoading,
}: {
  agents: Agent[]
  onSubmit: (rule: Omit<ForwardRule, 'id' | 'enabled' | 'createdAt' | 'trafficUsed'>) => void
  isLoading: boolean
}) {
  const [form, setForm] = useState<CreateRuleForm>({
    name: '',
    type: 'cloud-agent',
    protocol: 'tcp',
    sourceAgentId: '',
    listenPort: '',
    targetAgentId: '',
    targetHost: '127.0.0.1',
    targetPort: '',
    rateLimit: '',
    trafficLimit: '',
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    // Convert MB/s to bytes/s, GB to bytes
    const rateLimitBytes = form.rateLimit ? parseFloat(form.rateLimit) * 1024 * 1024 : 0
    const trafficLimitBytes = form.trafficLimit ? parseFloat(form.trafficLimit) * 1024 * 1024 * 1024 : 0
    
    // Determine which fields to include based on type
    const needsSourceAgent = form.type === 'agent-cloud' || form.type === 'agent-agent'
    const needsTargetAgent = form.type === 'cloud-agent' || form.type === 'agent-agent'
    
    onSubmit({
      name: form.name,
      type: form.type,
      protocol: form.protocol,
      sourceAgentId: needsSourceAgent ? form.sourceAgentId : undefined,
      listenPort: parseInt(form.listenPort),
      targetAgentId: needsTargetAgent ? form.targetAgentId : undefined,
      targetHost: form.targetHost,
      targetPort: parseInt(form.targetPort),
      rateLimit: rateLimitBytes,
      trafficLimit: trafficLimitBytes,
    })
  }

  return (
    <form onSubmit={handleSubmit}>
      <DialogHeader>
        <DialogTitle>添加转发规则</DialogTitle>
        <DialogDescription>
          配置端口转发规则，将流量转发到目标 Agent
        </DialogDescription>
      </DialogHeader>
      <div className="grid gap-4 py-4">
        <div className="grid gap-2">
          <Label>规则名称</Label>
          <Input
            placeholder="我的转发规则"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div className="grid gap-2">
            <Label>转发类型</Label>
            <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v as ForwardType })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="cloud-direct">云端直连 (Cloud → 目标服务器)</SelectItem>
                <SelectItem value="cloud-agent">云端到客户端 (Cloud → Agent → 目标)</SelectItem>
                <SelectItem value="agent-cloud">客户端到云端 (Agent → Cloud → 目标)</SelectItem>
                <SelectItem value="agent-agent">客户端到客户端 (Agent → Agent)</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label>协议</Label>
            <Select value={form.protocol} onValueChange={(v) => setForm({ ...form, protocol: v as 'tcp' | 'udp' })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tcp">TCP</SelectItem>
                <SelectItem value="udp">UDP</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        {(form.type === 'agent-cloud' || form.type === 'agent-agent') && (
          <div className="grid gap-2">
            <Label>源 Agent（监听端）</Label>
            <Select value={form.sourceAgentId} onValueChange={(v) => setForm({ ...form, sourceAgentId: v })}>
              <SelectTrigger>
                <SelectValue placeholder="选择源 Agent" />
              </SelectTrigger>
              <SelectContent>
                {agents.filter(a => a.online).map((agent) => (
                  <SelectItem key={agent.id} value={agent.id}>{agent.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="grid gap-2">
          <Label>监听端口</Label>
          <Input
            type="number"
            placeholder="8080"
            value={form.listenPort}
            onChange={(e) => setForm({ ...form, listenPort: e.target.value })}
          />
        </div>
        {(form.type === 'cloud-agent' || form.type === 'agent-agent') && (
          <div className="grid gap-2">
            <Label>目标 Agent（转发端）</Label>
            <Select value={form.targetAgentId} onValueChange={(v) => setForm({ ...form, targetAgentId: v })}>
              <SelectTrigger>
                <SelectValue placeholder="选择目标 Agent" />
              </SelectTrigger>
              <SelectContent>
                {agents.filter(a => a.online).map((agent) => (
                  <SelectItem key={agent.id} value={agent.id}>{agent.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="grid grid-cols-2 gap-4">
          <div className="grid gap-2">
            <Label>目标主机</Label>
            <Input
              placeholder="127.0.0.1"
              value={form.targetHost}
              onChange={(e) => setForm({ ...form, targetHost: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>目标端口</Label>
            <Input
              type="number"
              placeholder="80"
              value={form.targetPort}
              onChange={(e) => setForm({ ...form, targetPort: e.target.value })}
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div className="grid gap-2">
            <Label>速率限制 (MB/s)</Label>
            <Input
              type="number"
              placeholder="不限制"
              value={form.rateLimit}
              onChange={(e) => setForm({ ...form, rateLimit: e.target.value })}
            />
          </div>
          <div className="grid gap-2">
            <Label>流量限制 (GB)</Label>
            <Input
              type="number"
              placeholder="不限制"
              value={form.trafficLimit}
              onChange={(e) => setForm({ ...form, trafficLimit: e.target.value })}
            />
          </div>
        </div>
      </div>
      <DialogFooter>
        <Button type="submit" disabled={isLoading}>
          {isLoading ? '创建中...' : '创建规则'}
        </Button>
      </DialogFooter>
    </form>
  )
}

