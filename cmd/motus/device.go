package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// deviceOwnerEmail returns the email of the user a new device is assigned
// to: the --user flag, else the owner of auto-created GPS devices.
func deviceOwnerEmail(flag string) string {
	if flag != "" {
		return flag
	}
	return loadConfig().Device.AutoCreateDefaultUser
}

func newDeviceAddCmd() *cobra.Command {
	var uniqueID, name, protocol, userEmail string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new device",
		Long: "Register a new device and assign it to a user. Devices are only " +
			"visible in the UI to the users they are assigned to.",
		Run: func(cmd *cobra.Command, args []string) {
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				owner := deviceOwnerEmail(userEmail)
				user, err := repository.NewUserRepository(pool).GetByEmail(ctx, owner)
				if err != nil {
					fatalFn("user not found; pass --user with an existing user's email",
						slog.String("user", owner), slog.Any("error", err))
					return
				}

				device := &model.Device{
					UniqueID: uniqueID,
					Name:     name,
					Protocol: protocol,
					Status:   "offline",
				}
				if err := repository.NewDeviceRepository(pool).Create(ctx, device, user.ID); err != nil {
					if strings.Contains(err.Error(), "duplicate key") {
						fatal("device already exists", slog.String("uniqueID", uniqueID))
					}
					fatal("failed to create device", slog.Any("error", err))
				}

				fmt.Printf("Created device: id=%d, unique_id=%s, name=%s, protocol=%s, owner=%s\n",
					device.ID, uniqueID, name, protocol, user.Email)
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&uniqueID, "unique-id", "", "Device unique identifier")
	f.StringVar(&name, "name", "", "Device display name")
	f.StringVar(&protocol, "protocol", "h02", "Device protocol: h02, watch")
	f.StringVar(&userEmail, "user", "", "Email of the user to assign the device to "+
		"(default: $MOTUS_DEVICE_AUTO_CREATE_USER or admin@motus.local)")
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
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				devices, err := repository.NewDeviceRepository(pool).GetAll(ctx)
				if err != nil {
					fatal("failed to list devices", slog.Any("error", err))
				}

				if len(devices) == 0 {
					fmt.Println("No devices found.")
					return
				}

				if filter != "" {
					devices = filterList(devices, filter, deviceFilters)
					if len(devices) == 0 {
						fmt.Println("No devices match the filter.")
						return
					}
				}

				sortList(devices, sortField, deviceSorts)

				items := make([]map[string]any, len(devices))
				rows := make([][]string, len(devices))
				for i, d := range devices {
					item := map[string]any{
						"id":       d.ID,
						"uniqueId": d.UniqueID,
						"name":     d.Name,
						"protocol": d.Protocol,
						"status":   d.Status,
					}
					lastUpdate := "-"
					if d.LastUpdate != nil {
						item["lastUpdate"] = d.LastUpdate.Format(time.RFC3339)
						lastUpdate = d.LastUpdate.Format("2006-01-02 15:04")
					}
					items[i] = item
					rows[i] = []string{fmt.Sprint(d.ID), d.UniqueID, d.Name, d.Protocol, d.Status, lastUpdate}
				}
				headers := []string{"ID", "UNIQUE ID", "NAME", "PROTOCOL", "STATUS", "LAST UPDATE"}
				render(output, items, headers, rows)
			})
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
			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				tag, err := pool.Exec(ctx, `DELETE FROM devices WHERE unique_id = $1`, uniqueID)
				if err != nil {
					fatal("failed to delete device", slog.Any("error", err))
				}
				if tag.RowsAffected() == 0 {
					fmt.Fprintf(os.Stderr, "No device found with unique_id %q\n", uniqueID)
					os.Exit(1)
				}
			})

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

			withDB(func(ctx context.Context, pool *pgxpool.Pool) {
				deviceRepo := repository.NewDeviceRepository(pool)
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
			})
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

var deviceFilters = map[string]listFilter[model.Device]{
	"status":    {exact: true, get: func(d model.Device) string { return d.Status }},
	"protocol":  {exact: true, get: func(d model.Device) string { return d.Protocol }},
	"name":      {get: func(d model.Device) string { return d.Name }},
	"unique-id": {get: func(d model.Device) string { return d.UniqueID }},
	"uniqueid":  {get: func(d model.Device) string { return d.UniqueID }},
}

var deviceSorts = map[string]func(a, b model.Device) int{
	"id":        by(func(d model.Device) int64 { return d.ID }),
	"name":      by(func(d model.Device) string { return d.Name }),
	"unique-id": by(func(d model.Device) string { return d.UniqueID }),
	"uniqueid":  by(func(d model.Device) string { return d.UniqueID }),
	"status":    by(func(d model.Device) string { return d.Status }),
	"protocol":  by(func(d model.Device) string { return d.Protocol }),
}
