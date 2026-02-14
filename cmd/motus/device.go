package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

func newDeviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device",
		Short: "Manage devices",
	}
	cmd.AddCommand(
		newDeviceAddCmd(),
		newDeviceListCmd(),
		newDeviceDeleteCmd(),
		newDeviceUpdateCmd(),
	)
	return cmd
}

func newDeviceAddCmd() *cobra.Command {
	var uniqueID, name, protocol string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new device",
		Run: func(cmd *cobra.Command, args []string) {
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var deviceID int64
			err = pool.QueryRow(ctx, `
				INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at)
				VALUES ($1, $2, $3, 'offline', NOW(), NOW())
				RETURNING id
			`, uniqueID, name, protocol).Scan(&deviceID)
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") {
					fatal("device already exists", slog.String("uniqueID", uniqueID))
				}
				fatal("failed to create device", slog.Any("error", err))
			}

			fmt.Printf("Created device: id=%d, unique_id=%s, name=%s, protocol=%s\n",
				deviceID, uniqueID, name, protocol)
		},
	}

	f := cmd.Flags()
	f.StringVar(&uniqueID, "unique-id", "", "Device unique identifier")
	f.StringVar(&name, "name", "", "Device display name")
	f.StringVar(&protocol, "protocol", "h02", "Device protocol: h02, watch")
	_ = cmd.MarkFlagRequired("unique-id")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newDeviceListCmd() *cobra.Command {
	var output, filter, sortField string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all devices",
		Run: func(cmd *cobra.Command, args []string) {
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			deviceRepo := repository.NewDeviceRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			devices, err := deviceRepo.GetAll(ctx)
			if err != nil {
				fatal("failed to list devices", slog.Any("error", err))
			}

			if len(devices) == 0 {
				fmt.Println("No devices found.")
				return
			}

			if filter != "" {
				devices = filterDevices(devices, filter)
				if len(devices) == 0 {
					fmt.Println("No devices match the filter.")
					return
				}
			}

			sortDevices(devices, sortField)

			switch output {
			case "json":
				items := make([]map[string]interface{}, len(devices))
				for i, d := range devices {
					item := map[string]interface{}{
						"id":       d.ID,
						"uniqueId": d.UniqueID,
						"name":     d.Name,
						"protocol": d.Protocol,
						"status":   d.Status,
					}
					if d.LastUpdate != nil {
						item["lastUpdate"] = d.LastUpdate.Format(time.RFC3339)
					}
					items[i] = item
				}
				printJSON(items)
			case "csv":
				headers := []string{"ID", "UniqueID", "Name", "Protocol", "Status", "LastUpdate"}
				rows := make([][]string, len(devices))
				for i, d := range devices {
					lastUpdate := ""
					if d.LastUpdate != nil {
						lastUpdate = d.LastUpdate.Format("2006-01-02 15:04:05")
					}
					rows[i] = []string{
						fmt.Sprint(d.ID), d.UniqueID, d.Name, d.Protocol, d.Status, lastUpdate,
					}
				}
				printCSV(headers, rows)
			default:
				tw := NewTableWriter(os.Stdout)
				tw.WriteHeader("ID", "UNIQUE ID", "NAME", "PROTOCOL", "STATUS", "LAST UPDATE")
				for _, d := range devices {
					lastUpdate := "-"
					if d.LastUpdate != nil {
						lastUpdate = d.LastUpdate.Format("2006-01-02 15:04")
					}
					tw.WriteRow(fmt.Sprint(d.ID), d.UniqueID, d.Name, d.Protocol, d.Status, lastUpdate)
				}
				tw.Flush()
			}
		},
	}

	f := cmd.Flags()
	f.StringVar(&output, "output", "table", "Output format: table, json, csv")
	f.StringVar(&filter, "filter", "", "Filter by field=value (e.g. status=online)")
	f.StringVar(&sortField, "sort", "id", "Sort by field: id, name, unique-id, status, protocol")

	return cmd
}

func newDeviceDeleteCmd() *cobra.Command {
	var uniqueID string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a device by unique ID",
		Run: func(cmd *cobra.Command, args []string) {
			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			tag, err := pool.Exec(ctx, `DELETE FROM devices WHERE unique_id = $1`, uniqueID)
			if err != nil {
				fatal("failed to delete device", slog.Any("error", err))
			}
			if tag.RowsAffected() == 0 {
				fmt.Fprintf(os.Stderr, "No device found with unique_id %q\n", uniqueID)
				os.Exit(1)
			}

			fmt.Printf("Deleted device: %s\n", uniqueID)
		},
	}

	cmd.Flags().StringVar(&uniqueID, "unique-id", "", "Unique ID of device to delete")
	_ = cmd.MarkFlagRequired("unique-id")

	return cmd
}

func newDeviceUpdateCmd() *cobra.Command {
	var uniqueID, name, protocol string
	var speedLimit float64
	var clearSpeedLimit bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a device's name, protocol, or speed limit",
		Run: func(cmd *cobra.Command, args []string) {
			if name == "" && protocol == "" && speedLimit == 0 && !clearSpeedLimit {
				fmt.Fprintln(os.Stderr, "Error: at least one of --name, --protocol, --speed-limit, or --clear-speed-limit must be specified")
				os.Exit(1)
			}
			if speedLimit > 0 && clearSpeedLimit {
				fmt.Fprintln(os.Stderr, "Error: --speed-limit and --clear-speed-limit are mutually exclusive")
				os.Exit(1)
			}

			pool, err := connectDBFn()
			if err != nil {
				fatal("database connection failed", slog.Any("error", err))
			}
			defer pool.Close()

			deviceRepo := repository.NewDeviceRepository(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			d, err := deviceRepo.GetByUniqueID(ctx, uniqueID)
			if err != nil {
				fatal("device not found", slog.String("uniqueID", uniqueID))
			}

			if name != "" {
				d.Name = name
			}
			if protocol != "" {
				d.Protocol = protocol
			}
			if clearSpeedLimit {
				d.SpeedLimit = nil
			} else if speedLimit > 0 {
				d.SpeedLimit = &speedLimit
			}

			if err := deviceRepo.Update(ctx, d); err != nil {
				fatal("failed to update device", slog.Any("error", err))
			}

			speedStr := "-"
			if d.SpeedLimit != nil {
				speedStr = fmt.Sprintf("%.1f km/h", *d.SpeedLimit)
			}
			fmt.Printf("Updated device: id=%d, unique_id=%s, name=%s, protocol=%s, speed_limit=%s\n",
				d.ID, d.UniqueID, d.Name, d.Protocol, speedStr)
		},
	}

	f := cmd.Flags()
	f.StringVar(&uniqueID, "unique-id", "", "Device unique ID")
	f.StringVar(&name, "name", "", "New display name")
	f.StringVar(&protocol, "protocol", "", "New protocol")
	f.Float64Var(&speedLimit, "speed-limit", 0, "Speed limit in km/h (must be > 0)")
	f.BoolVar(&clearSpeedLimit, "clear-speed-limit", false, "Clear the speed limit")
	_ = cmd.MarkFlagRequired("unique-id")

	return cmd
}

// filterDevices returns devices matching the given field=value filter.
func filterDevices(devices []model.Device, filter string) []model.Device {
	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 {
		fatalFn("invalid filter format (expected field=value)", slog.String("filter", filter))
		return nil
	}
	field, value := strings.ToLower(parts[0]), strings.ToLower(parts[1])

	var result []model.Device
	for _, d := range devices {
		switch field {
		case "status":
			if strings.ToLower(d.Status) == value {
				result = append(result, d)
			}
		case "protocol":
			if strings.ToLower(d.Protocol) == value {
				result = append(result, d)
			}
		case "name":
			if strings.Contains(strings.ToLower(d.Name), value) {
				result = append(result, d)
			}
		case "unique-id", "uniqueid":
			if strings.Contains(strings.ToLower(d.UniqueID), value) {
				result = append(result, d)
			}
		default:
			fatalFn("unknown filter field (supported: status, protocol, name, unique-id)",
				slog.String("field", field))
			return nil
		}
	}
	return result
}

// sortDevices sorts devices in-place by the given field.
func sortDevices(devices []model.Device, field string) {
	switch strings.ToLower(field) {
	case "name":
		sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	case "unique-id", "uniqueid":
		sort.Slice(devices, func(i, j int) bool { return devices[i].UniqueID < devices[j].UniqueID })
	case "status":
		sort.Slice(devices, func(i, j int) bool { return devices[i].Status < devices[j].Status })
	case "protocol":
		sort.Slice(devices, func(i, j int) bool { return devices[i].Protocol < devices[j].Protocol })
	default: // "id" or unrecognized
		sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	}
}
