	POD=$(kubectl get pod -n projectcontour -l app=contour -o name | head -1)
	kubectl logs "$POD" -n projectcontour