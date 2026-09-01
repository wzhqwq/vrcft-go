import type {ProblemView} from '../../presentation/problem.js'

export type ModuleStatus = 'loading' | 'ready' | 'stale' | 'problem'

export interface ModuleState<TSnapshot> {
  readonly status: ModuleStatus
  readonly snapshot: TSnapshot | null
  readonly revision: number | null
  readonly updatedAt: string | null
  readonly problem: ProblemView | null
}
