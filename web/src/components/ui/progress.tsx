import { Progress as ProgressPrimitive } from 'radix-ui'
import * as React from 'react'

import { cn } from '@/utils/cn'

function Progress({
	className,
	value,
	...props
}: React.ComponentProps<typeof ProgressPrimitive.Root>) {
	const percent = Math.max(0, Math.min(100, value ?? 0))

	return (
		<ProgressPrimitive.Root
			data-slot="progress"
			className={cn(
				'relative h-2 w-full overflow-hidden rounded-full bg-primary/20',
				className,
			)}
			{...props}
		>
			<ProgressPrimitive.Indicator
				data-slot="progress-indicator"
				className="h-full bg-primary transition-all"
				style={{ width: `${percent}%` }}
			/>
		</ProgressPrimitive.Root>
	)
}

export { Progress }
