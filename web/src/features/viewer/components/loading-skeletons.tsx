import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function PVCListSkeleton() {
	return (
		<Card>
			<CardHeader>
				<Skeleton className="h-6 w-40" />
				<Skeleton className="h-4 w-72" />
			</CardHeader>
			<CardContent className="flex flex-col gap-3">
				{Array.from({ length: 5 }, (_, index) => (
					<Skeleton className="h-14 w-full" key={index} />
				))}
			</CardContent>
		</Card>
	)
}
