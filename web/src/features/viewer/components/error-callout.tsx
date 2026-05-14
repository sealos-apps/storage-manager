import type { ReactNode } from 'react'

import { AlertTriangle } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

interface ErrorCalloutProps {
	children: ReactNode
	title: string
}

export function ErrorCallout({ children, title }: ErrorCalloutProps) {
	return (
		<Alert variant="destructive">
			<AlertTriangle />
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>{children}</AlertDescription>
		</Alert>
	)
}
